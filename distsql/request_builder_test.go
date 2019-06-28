// Copyright 2018 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package distsql

import (
	"os"
	"testing"

	. "github.com/pingcap/check"
	"github.com/pingcap/tidb/kv"
	"github.com/pingcap/tidb/sessionctx"
	"github.com/pingcap/tidb/sessionctx/stmtctx"
	"github.com/pingcap/tidb/sessionctx/variable"
	"github.com/pingcap/tidb/tablecodec"
	"github.com/pingcap/tidb/types"
	"github.com/pingcap/tidb/util/codec"
	"github.com/pingcap/tidb/util/logutil"
	"github.com/pingcap/tidb/util/memory"
	"github.com/pingcap/tidb/util/mock"
	"github.com/pingcap/tidb/util/ranger"
	"github.com/pingcap/tidb/util/testleak"
	"github.com/pingcap/tipb/go-tipb"
)

var _ = Suite(&testSuite{})

func TestT(t *testing.T) {
	CustomVerboseFlag = true
	logLevel := os.Getenv("log_level")
	logutil.InitLogger(logutil.NewLogConfig(logLevel, logutil.DefaultLogFormat, "", logutil.EmptyFileLogConfig, false))
	TestingT(t)
}

var _ = Suite(&testSuite{})

type testSuite struct {
	sctx sessionctx.Context
}

func (s *testSuite) SetUpSuite(c *C) {
	ctx := mock.NewContext()
	ctx.GetSessionVars().StmtCtx = &stmtctx.StatementContext{
		MemTracker: memory.NewTracker("testSuite", variable.DefTiDBMemQuotaDistSQL),
	}
	ctx.Store = &mock.Store{
		Client: &mock.Client{
			MockResponse: &mockResponse{
				batch: 1,
				total: 2,
			},
		},
	}
	s.sctx = ctx
}

func (s *testSuite) TearDownSuite(c *C) {
}

func (s *testSuite) SetUpTest(c *C) {
	testleak.BeforeTest()
	ctx := s.sctx.(*mock.Context)
	store := ctx.Store.(*mock.Store)
	store.Client = &mock.Client{
		MockResponse: &mockResponse{
			batch: 1,
			total: 2,
		},
	}
}

func (s *testSuite) TearDownTest(c *C) {
	testleak.AfterTest(c)()
}

type handleRange struct {
	start int64
	end   int64
}

func (s *testSuite) getExpectedRanges(tid int64, hrs []*handleRange) []kv.KeyRange {
	krs := make([]kv.KeyRange, 0, len(hrs))
	for _, hr := range hrs {
		low := codec.EncodeInt(nil, hr.start)
		high := codec.EncodeInt(nil, hr.end)
		high = []byte(kv.Key(high).PrefixNext())
		startKey := tablecodec.EncodeRowKey(tid, low)
		endKey := tablecodec.EncodeRowKey(tid, high)
		krs = append(krs, kv.KeyRange{StartKey: startKey, EndKey: endKey})
	}
	return krs
}

func (s *testSuite) TestTableHandlesToKVRanges(c *C) {
	handles := []int64{0, 2, 3, 4, 5, 10, 11, 100, 9223372036854775806, 9223372036854775807}

	// Build expected key ranges.
	hrs := make([]*handleRange, 0, len(handles))
	hrs = append(hrs, &handleRange{start: 0, end: 0})
	hrs = append(hrs, &handleRange{start: 2, end: 5})
	hrs = append(hrs, &handleRange{start: 10, end: 11})
	hrs = append(hrs, &handleRange{start: 100, end: 100})
	hrs = append(hrs, &handleRange{start: 9223372036854775806, end: 9223372036854775807})

	// Build key ranges.
	expect := s.getExpectedRanges(1, hrs)
	actual := TableHandlesToKVRanges(1, handles)

	// Compare key ranges and expected key ranges.
	c.Assert(len(actual), Equals, len(expect))
	for i := range actual {
		c.Assert(actual[i].StartKey, DeepEquals, expect[i].StartKey)
		c.Assert(actual[i].EndKey, DeepEquals, expect[i].EndKey)
	}
}

func (s *testSuite) TestTableRangesToKVRanges(c *C) {
	ranges := []*ranger.Range{
		{
			LowVal:  []types.Datum{types.NewIntDatum(1)},
			HighVal: []types.Datum{types.NewIntDatum(2)},
		},
		{
			LowVal:      []types.Datum{types.NewIntDatum(2)},
			HighVal:     []types.Datum{types.NewIntDatum(4)},
			LowExclude:  true,
			HighExclude: true,
		},
		{
			LowVal:      []types.Datum{types.NewIntDatum(4)},
			HighVal:     []types.Datum{types.NewIntDatum(19)},
			HighExclude: true,
		},
		{
			LowVal:     []types.Datum{types.NewIntDatum(19)},
			HighVal:    []types.Datum{types.NewIntDatum(32)},
			LowExclude: true,
		},
		{
			LowVal:     []types.Datum{types.NewIntDatum(34)},
			HighVal:    []types.Datum{types.NewIntDatum(34)},
			LowExclude: true,
		},
	}

	actual := TableRangesToKVRanges(13, ranges, nil)
	expect := []kv.KeyRange{
		{
			StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1},
			EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3},
		},
		{
			StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3},
			EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x4},
		},
		{
			StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x4},
			EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x13},
		},
		{
			StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x14},
			EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x21},
		},
		{
			StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x23},
			EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x23},
		},
	}
	for i := 0; i < len(actual); i++ {
		c.Assert(actual[i], DeepEquals, expect[i])
	}
}

func (s *testSuite) TestIndexRangesToKVRanges(c *C) {
	ranges := []*ranger.Range{
		{
			LowVal:  []types.Datum{types.NewIntDatum(1)},
			HighVal: []types.Datum{types.NewIntDatum(2)},
		},
		{
			LowVal:      []types.Datum{types.NewIntDatum(2)},
			HighVal:     []types.Datum{types.NewIntDatum(4)},
			LowExclude:  true,
			HighExclude: true,
		},
		{
			LowVal:      []types.Datum{types.NewIntDatum(4)},
			HighVal:     []types.Datum{types.NewIntDatum(19)},
			HighExclude: true,
		},
		{
			LowVal:     []types.Datum{types.NewIntDatum(19)},
			HighVal:    []types.Datum{types.NewIntDatum(32)},
			LowExclude: true,
		},
		{
			LowVal:     []types.Datum{types.NewIntDatum(34)},
			HighVal:    []types.Datum{types.NewIntDatum(34)},
			LowExclude: true,
		},
	}

	expect := []kv.KeyRange{
		{
			StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x69, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x3, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1},
			EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x69, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x3, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3},
		},
		{
			StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x69, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x3, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3},
			EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x69, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x3, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x4},
		},
		{
			StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x69, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x3, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x4},
			EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x69, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x3, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x13},
		},
		{
			StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x69, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x3, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x14},
			EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x69, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x3, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x21},
		},
		{
			StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x69, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x3, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x23},
			EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x69, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x3, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x23},
		},
	}

	actual, err := IndexRangesToKVRanges(new(stmtctx.StatementContext), 12, 15, ranges, nil)
	c.Assert(err, IsNil)
	for i := range actual {
		c.Assert(actual[i], DeepEquals, expect[i])
	}
}

func (s *testSuite) TestRequestBuilder1(c *C) {
	ranges := []*ranger.Range{
		{
			LowVal:  []types.Datum{types.NewIntDatum(1)},
			HighVal: []types.Datum{types.NewIntDatum(2)},
		},
		{
			LowVal:      []types.Datum{types.NewIntDatum(2)},
			HighVal:     []types.Datum{types.NewIntDatum(4)},
			LowExclude:  true,
			HighExclude: true,
		},
		{
			LowVal:      []types.Datum{types.NewIntDatum(4)},
			HighVal:     []types.Datum{types.NewIntDatum(19)},
			HighExclude: true,
		},
		{
			LowVal:     []types.Datum{types.NewIntDatum(19)},
			HighVal:    []types.Datum{types.NewIntDatum(32)},
			LowExclude: true,
		},
		{
			LowVal:     []types.Datum{types.NewIntDatum(34)},
			HighVal:    []types.Datum{types.NewIntDatum(34)},
			LowExclude: true,
		},
	}

	actual, err := (&RequestBuilder{}).SetTableRanges(12, ranges, nil).
		SetDAGRequest(&tipb.DAGRequest{}).
		SetDesc(false).
		SetKeepOrder(false).
		SetFromSessionVars(variable.NewSessionVars()).
		Build()
	c.Assert(err, IsNil)
	expect := &kv.Request{
		Tp:      103,
		StartTs: 0x0,
		Data:    []uint8{0x8, 0x0, 0x18, 0x0, 0x20, 0x0, 0x40, 0x0, 0x5a, 0x0},
		KeyRanges: []kv.KeyRange{
			{
				StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1},
				EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3},
			},
			{
				StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3},
				EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x4},
			},
			{
				StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x4},
				EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x13},
			},
			{
				StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x14},
				EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x21},
			},
			{
				StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x23},
				EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x23},
			},
		},
		KeepOrder:      false,
		Desc:           false,
		Concurrency:    15,
		IsolationLevel: 0,
		Priority:       0,
		NotFillCache:   false,
		SyncLog:        false,
		Streaming:      false,
	}
	c.Assert(actual, DeepEquals, expect)
}

func (s *testSuite) TestRequestBuilder2(c *C) {
	ranges := []*ranger.Range{
		{
			LowVal:  []types.Datum{types.NewIntDatum(1)},
			HighVal: []types.Datum{types.NewIntDatum(2)},
		},
		{
			LowVal:      []types.Datum{types.NewIntDatum(2)},
			HighVal:     []types.Datum{types.NewIntDatum(4)},
			LowExclude:  true,
			HighExclude: true,
		},
		{
			LowVal:      []types.Datum{types.NewIntDatum(4)},
			HighVal:     []types.Datum{types.NewIntDatum(19)},
			HighExclude: true,
		},
		{
			LowVal:     []types.Datum{types.NewIntDatum(19)},
			HighVal:    []types.Datum{types.NewIntDatum(32)},
			LowExclude: true,
		},
		{
			LowVal:     []types.Datum{types.NewIntDatum(34)},
			HighVal:    []types.Datum{types.NewIntDatum(34)},
			LowExclude: true,
		},
	}

	actual, err := (&RequestBuilder{}).SetIndexRanges(new(stmtctx.StatementContext), 12, 15, ranges).
		SetDAGRequest(&tipb.DAGRequest{}).
		SetDesc(false).
		SetKeepOrder(false).
		SetFromSessionVars(variable.NewSessionVars()).
		Build()
	c.Assert(err, IsNil)
	expect := &kv.Request{
		Tp:      103,
		StartTs: 0x0,
		Data:    []uint8{0x8, 0x0, 0x18, 0x0, 0x20, 0x0, 0x40, 0x0, 0x5a, 0x0},
		KeyRanges: []kv.KeyRange{
			{
				StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x69, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x3, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1},
				EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x69, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x3, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3},
			},
			{
				StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x69, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x3, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3},
				EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x69, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x3, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x4},
			},
			{
				StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x69, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x3, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x4},
				EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x69, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x3, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x13},
			},
			{
				StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x69, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x3, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x14},
				EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x69, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x3, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x21},
			},
			{
				StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x69, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x3, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x23},
				EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x5f, 0x69, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x3, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x23},
			},
		},
		KeepOrder:      false,
		Desc:           false,
		Concurrency:    15,
		IsolationLevel: 0,
		Priority:       0,
		NotFillCache:   false,
		SyncLog:        false,
		Streaming:      false,
	}
	c.Assert(actual, DeepEquals, expect)
}

func (s *testSuite) TestRequestBuilder3(c *C) {
	handles := []int64{0, 2, 3, 4, 5, 10, 11, 100}

	actual, err := (&RequestBuilder{}).SetTableHandles(15, handles).
		SetDAGRequest(&tipb.DAGRequest{}).
		SetDesc(false).
		SetKeepOrder(false).
		SetFromSessionVars(variable.NewSessionVars()).
		Build()
	c.Assert(err, IsNil)
	expect := &kv.Request{
		Tp:      103,
		StartTs: 0x0,
		Data:    []uint8{0x8, 0x0, 0x18, 0x0, 0x20, 0x0, 0x40, 0x0, 0x5a, 0x0},
		KeyRanges: []kv.KeyRange{
			{
				StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
				EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1},
			},
			{
				StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2},
				EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x6},
			},
			{
				StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xa},
				EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc},
			},
			{
				StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x64},
				EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x65},
			},
		},
		KeepOrder:      false,
		Desc:           false,
		Concurrency:    15,
		IsolationLevel: 0,
		Priority:       0,
		NotFillCache:   false,
		SyncLog:        false,
		Streaming:      false,
	}
	c.Assert(actual, DeepEquals, expect)
}

func (s *testSuite) TestRequestBuilder4(c *C) {
	keyRanges := []kv.KeyRange{
		{
			StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1},
		},
		{
			StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2},
			EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x6},
		},
		{
			StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xa},
			EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc},
		},
		{
			StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x64},
			EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x65},
		},
	}

	actual, err := (&RequestBuilder{}).SetKeyRanges(keyRanges).
		SetDAGRequest(&tipb.DAGRequest{}).
		SetDesc(false).
		SetKeepOrder(false).
		SetStreaming(true).
		SetFromSessionVars(variable.NewSessionVars()).
		Build()
	c.Assert(err, IsNil)
	expect := &kv.Request{
		Tp:             103,
		StartTs:        0x0,
		Data:           []uint8{0x8, 0x0, 0x18, 0x0, 0x20, 0x0, 0x40, 0x0, 0x5a, 0x0},
		KeyRanges:      keyRanges,
		KeepOrder:      false,
		Desc:           false,
		Concurrency:    15,
		IsolationLevel: 0,
		Priority:       0,
		Streaming:      true,
		NotFillCache:   false,
		SyncLog:        false,
	}
	c.Assert(actual, DeepEquals, expect)
}

func (s *testSuite) TestRequestBuilder5(c *C) {
	keyRanges := []kv.KeyRange{
		{
			StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1},
		},
		{
			StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2},
			EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x6},
		},
		{
			StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xa},
			EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc},
		},
		{
			StartKey: kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x64},
			EndKey:   kv.Key{0x74, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x5f, 0x72, 0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x65},
		},
	}

	actual, err := (&RequestBuilder{}).SetKeyRanges(keyRanges).
		SetAnalyzeRequest(&tipb.AnalyzeReq{}).
		SetKeepOrder(true).
		SetConcurrency(15).
		Build()
	c.Assert(err, IsNil)
	expect := &kv.Request{
		Tp:             104,
		StartTs:        0x0,
		Data:           []uint8{0x8, 0x0, 0x10, 0x0, 0x18, 0x0, 0x20, 0x0},
		KeyRanges:      keyRanges,
		KeepOrder:      true,
		Desc:           false,
		Concurrency:    15,
		IsolationLevel: kv.RC,
		Priority:       1,
		NotFillCache:   true,
		SyncLog:        false,
		Streaming:      false,
	}
	c.Assert(actual, DeepEquals, expect)
}

func (s *testSuite) TestRequestBuilder6(c *C) {
	keyRanges := []kv.KeyRange{
		{
			StartKey: kv.Key{0x00, 0x01},
			EndKey:   kv.Key{0x02, 0x03},
		},
	}

	concurrency := 10

	actual, err := (&RequestBuilder{}).SetKeyRanges(keyRanges).
		SetChecksumRequest(&tipb.ChecksumRequest{}).
		SetConcurrency(concurrency).
		Build()
	c.Assert(err, IsNil)

	expect := &kv.Request{
		Tp:             105,
		StartTs:        0x0,
		Data:           []uint8{0x8, 0x0, 0x10, 0x0, 0x18, 0x0},
		KeyRanges:      keyRanges,
		KeepOrder:      false,
		Desc:           false,
		Concurrency:    concurrency,
		IsolationLevel: 0,
		Priority:       0,
		NotFillCache:   true,
		SyncLog:        false,
		Streaming:      false,
	}

	c.Assert(actual, DeepEquals, expect)
}
