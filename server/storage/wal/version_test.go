// Copyright 2021 The etcd Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package wal

import (
	"fmt"
	"testing"

	"github.com/coreos/go-semver/semver"
	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.etcd.io/etcd/api/v3/etcdserverpb"
	"go.etcd.io/etcd/api/v3/membershippb"
	"go.etcd.io/etcd/api/v3/version"
	"go.etcd.io/etcd/pkg/v3/pbutil"
	"go.etcd.io/raft/v3/raftpb"
)

func TestEtcdVersionFromEntry(t *testing.T) {
	raftReq := etcdserverpb.InternalRaftRequest{Header: &etcdserverpb.RequestHeader{AuthRevision: 1}}
	normalRequestData := pbutil.MustMarshal(&raftReq)

	downgradeVersionTestV3_6Req := etcdserverpb.InternalRaftRequest{DowngradeVersionTest: &etcdserverpb.DowngradeVersionTestRequest{Ver: "3.6.0"}}
	downgradeVersionTestV3_6Data := pbutil.MustMarshal(&downgradeVersionTestV3_6Req)

	downgradeVersionTestV3_7Req := etcdserverpb.InternalRaftRequest{DowngradeVersionTest: &etcdserverpb.DowngradeVersionTestRequest{Ver: "3.7.0"}}
	downgradeVersionTestV3_7Data := pbutil.MustMarshal(&downgradeVersionTestV3_7Req)

	confChange := raftpb.ConfChange{Type: raftpb.ConfChangeAddLearnerNode}
	confChangeData := pbutil.MustMarshal(&confChange)

	confChangeV2 := raftpb.ConfChangeV2{Transition: raftpb.ConfChangeTransitionJointExplicit}
	confChangeV2Data := pbutil.MustMarshal(&confChangeV2)

	tcs := []struct {
		name   string
		input  raftpb.Entry
		expect *semver.Version
	}{
		{
			name: "Using RequestHeader AuthRevision in NormalEntry implies v3.1",
			input: raftpb.Entry{
				Term:  1,
				Index: 2,
				Type:  raftpb.EntryNormal,
				Data:  normalRequestData,
			},
			expect: &version.V3_1,
		},
		{
			name: "Setting downgradeTest version to 3.6 implies version within WAL",
			input: raftpb.Entry{
				Term:  1,
				Index: 2,
				Type:  raftpb.EntryNormal,
				Data:  downgradeVersionTestV3_6Data,
			},
			expect: &version.V3_6,
		},
		{
			name: "Setting downgradeTest version to 3.7 implies version within WAL",
			input: raftpb.Entry{
				Term:  1,
				Index: 2,
				Type:  raftpb.EntryNormal,
				Data:  downgradeVersionTestV3_7Data,
			},
			expect: &version.V3_7,
		},
		{
			name: "Using ConfigChange implies v3.0",
			input: raftpb.Entry{
				Term:  1,
				Index: 2,
				Type:  raftpb.EntryConfChange,
				Data:  confChangeData,
			},
			expect: &version.V3_0,
		},
		{
			name: "Using ConfigChangeV2 implies v3.4",
			input: raftpb.Entry{
				Term:  1,
				Index: 2,
				Type:  raftpb.EntryConfChangeV2,
				Data:  confChangeV2Data,
			},
			expect: &version.V3_4,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			var maxVer *semver.Version
			err := visitEntry(tc.input, func(path protoreflect.FullName, ver *semver.Version) error {
				maxVer = maxVersion(maxVer, ver)
				return nil
			})
			require.NoError(t, err)
			assert.Equal(t, tc.expect, maxVer)
		})
	}
}

func TestEtcdVersionFromMessage(t *testing.T) {
	tcs := []struct {
		name   string
		input  proto.Message
		expect *semver.Version
	}{
		{
			name:   "Empty RequestHeader impies v3.0",
			input:  &etcdserverpb.RequestHeader{},
			expect: &version.V3_0,
		},
		{
			name:   "RequestHeader AuthRevision field set implies v3.5",
			input:  &etcdserverpb.RequestHeader{AuthRevision: 1},
			expect: &version.V3_1,
		},
		{
			name:   "RequestHeader Username set implies v3.0",
			input:  &etcdserverpb.RequestHeader{Username: "Alice"},
			expect: &version.V3_0,
		},
		{
			name:   "When two fields are set take higher version",
			input:  &etcdserverpb.RequestHeader{AuthRevision: 1, Username: "Alice"},
			expect: &version.V3_1,
		},
		{
			name:   "Setting a RequestHeader AuthRevision in subfield implies v3.1",
			input:  &etcdserverpb.InternalRaftRequest{Header: &etcdserverpb.RequestHeader{AuthRevision: 1}},
			expect: &version.V3_1,
		},
		{
			name:   "Setting a DowngradeInfoSetRequest implies v3.5",
			input:  &etcdserverpb.InternalRaftRequest{DowngradeInfoSet: &membershippb.DowngradeInfoSetRequest{}},
			expect: &version.V3_5,
		},
		{
			name:   "Enum CompareResult set to EQUAL implies v3.0",
			input:  &etcdserverpb.Compare{Result: etcdserverpb.Compare_EQUAL},
			expect: &version.V3_0,
		},
		{
			name:   "Enum CompareResult set to NOT_EQUAL implies v3.1",
			input:  &etcdserverpb.Compare{Result: etcdserverpb.Compare_NOT_EQUAL},
			expect: &version.V3_1,
		},
		{
			name:   "Oneof Compare version set implies v3.1",
			input:  &etcdserverpb.Compare{TargetUnion: &etcdserverpb.Compare_Version{}},
			expect: &version.V3_0,
		},
		{
			name:   "Oneof Compare lease set implies v3.3",
			input:  &etcdserverpb.Compare{TargetUnion: &etcdserverpb.Compare_Lease{}},
			expect: &version.V3_3,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			var maxVer *semver.Version
			err := visitMessage(proto.MessageReflect(tc.input), func(path protoreflect.FullName, ver *semver.Version) error {
				maxVer = maxVersion(maxVer, ver)
				return nil
			})
			require.NoError(t, err)
			assert.Equal(t, tc.expect, maxVer)
		})
	}
}

func TestEtcdVersionFromFieldOptionsString(t *testing.T) {
	tcs := []struct {
		input  string
		expect *semver.Version
	}{
		{
			input: "65001:0",
		},
		{
			input: `65001:0  65004:"NodeID"`,
		},
		{
			input: `[versionpb.XXX]:"3.5"`,
		},
		{
			input:  `[versionpb.etcd_version_msg]:"3.5"`,
			expect: &version.V3_5,
		},
		{
			input:  `[versionpb.etcd_version_enum]:"3.5"`,
			expect: &version.V3_5,
		},
		{
			input:  `[versionpb.etcd_version_field]:"3.5"`,
			expect: &version.V3_5,
		},
		{
			input:  `[versionpb.etcd_version_enum_value]:"3.5"`,
			expect: &version.V3_5,
		},
		{
			input:  `65001:0 [versionpb.etcd_version_msg]:"3.5"`,
			expect: &version.V3_5,
		},
		{
			input:  `65004:"NodeID" [versionpb.etcd_version_msg]:"3.5"`,
			expect: &version.V3_5,
		},
		{
			input:  `65004:"NodeID" [versionpb.etcd_version_enum]:"3.5"`,
			expect: &version.V3_5,
		},
		{
			input:  `[versionpb.other_field]:"NodeID" [versionpb.etcd_version_msg]:"3.5"`,
			expect: &version.V3_5,
		},
		{
			input:  `[versionpb.etcd_version_msg]:"3.5" 65001:0`,
			expect: &version.V3_5,
		},
		{
			input:  `[versionpb.etcd_version_msg]:"3.5" 65004:"NodeID"`,
			expect: &version.V3_5,
		},
		{
			input:  `[versionpb.etcd_version_msg]:"3.5" [versionpb.other_field]:"NodeID"`,
			expect: &version.V3_5,
		},
		{
			input:  `[versionpb.other_field]:"NodeID" [versionpb.etcd_version_msg]:"3.5" [versionpb.another_field]:"NodeID"`,
			expect: &version.V3_5,
		},
		{
			input:  `65001:0 [versionpb.etcd_version_msg]:"3.5" 65001:0"`,
			expect: &version.V3_5,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.input, func(t *testing.T) {
			ver, err := etcdVersionFromOptionsString(tc.input)
			require.NoError(t, err)
			assert.Equal(t, ver, tc.expect)
		})
	}
}

func TestMaxVersion(t *testing.T) {
	tcs := []struct {
		a, b, expect *semver.Version
	}{
		{
			a:      nil,
			b:      nil,
			expect: nil,
		},
		{
			a:      &version.V3_5,
			b:      nil,
			expect: &version.V3_5,
		},
		{
			a:      nil,
			b:      &version.V3_5,
			expect: &version.V3_5,
		},
		{
			a:      &version.V3_6,
			b:      &version.V3_5,
			expect: &version.V3_6,
		},
		{
			a:      &version.V3_5,
			b:      &version.V3_6,
			expect: &version.V3_6,
		},
	}
	for _, tc := range tcs {
		t.Run(fmt.Sprintf("%v %v %v", tc.a, tc.b, tc.expect), func(t *testing.T) {
			got := maxVersion(tc.a, tc.b)
			assert.Equal(t, got, tc.expect)
		})
	}
}
