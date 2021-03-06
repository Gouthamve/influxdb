package cluster_test

import (
	"fmt"
	"net"
	"time"

	"github.com/influxdata/influxdb/cluster"
	"github.com/influxdata/influxdb/influxql"
	"github.com/influxdata/influxdb/models"
	"github.com/influxdata/influxdb/services/meta"
	"github.com/influxdata/influxdb/tcp"
)

type metaClient struct {
	host string
}

func (m *metaClient) DataNode(nodeID uint64) (*meta.NodeInfo, error) {
	return &meta.NodeInfo{
		ID:      nodeID,
		TCPHost: m.host,
	}, nil
}

type testService struct {
	nodeID                    uint64
	ln                        net.Listener
	muxln                     net.Listener
	writeShardFunc            func(shardID uint64, points []models.Point) error
	createShardFunc           func(database, policy string, shardID uint64) error
	deleteDatabaseFunc        func(database string) error
	deleteMeasurementFunc     func(database, name string) error
	deleteSeriesFunc          func(database string, sources []influxql.Source, condition influxql.Expr) error
	deleteRetentionPolicyFunc func(database, name string) error
}

func newTestWriteService(f func(shardID uint64, points []models.Point) error) testService {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}

	mux := tcp.NewMux()
	muxln := mux.Listen(cluster.MuxHeader)
	go mux.Serve(ln)

	return testService{
		writeShardFunc: f,
		ln:             ln,
		muxln:          muxln,
	}
}

func (ts *testService) Close() {
	if ts.ln != nil {
		ts.ln.Close()
	}
}

type serviceResponses []serviceResponse
type serviceResponse struct {
	shardID uint64
	ownerID uint64
	points  []models.Point
}

func (t testService) WriteToShard(shardID uint64, points []models.Point) error {
	return t.writeShardFunc(shardID, points)
}

func (t testService) CreateShard(database, policy string, shardID uint64) error {
	return t.createShardFunc(database, policy, shardID)
}

func (t testService) DeleteDatabase(database string) error {
	return t.deleteDatabaseFunc(database)
}

func (t testService) DeleteMeasurement(database, name string) error {
	return t.deleteMeasurementFunc(database, name)
}

func (t testService) DeleteSeries(database string, sources []influxql.Source, condition influxql.Expr) error {
	return t.deleteSeriesFunc(database, sources, condition)
}

func (t testService) DeleteRetentionPolicy(database, name string) error {
	return t.deleteRetentionPolicyFunc(database, name)
}

func writeShardSuccess(shardID uint64, points []models.Point) error {
	responses <- &serviceResponse{
		shardID: shardID,
		points:  points,
	}
	return nil
}

func writeShardFail(shardID uint64, points []models.Point) error {
	return fmt.Errorf("failed to write")
}

func writeShardSlow(shardID uint64, points []models.Point) error {
	time.Sleep(1 * time.Second)
	return nil
}

var responses = make(chan *serviceResponse, 1024)

func (testService) ResponseN(n int) ([]*serviceResponse, error) {
	var a []*serviceResponse
	for {
		select {
		case r := <-responses:
			a = append(a, r)
			if len(a) == n {
				return a, nil
			}
		case <-time.After(time.Second):
			return a, fmt.Errorf("unexpected response count: expected: %d, actual: %d", n, len(a))
		}
	}
}
