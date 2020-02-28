package main

import (
	"context"
	"fmt"
	"log"
	"testing"

	"google.golang.org/grpc"

	bspb "github.com/brotherlogic/beerserver/proto"
	"github.com/golang/protobuf/jsonpb"
)

func TestBuildClient(t *testing.T) {
	s := Init()
	client, err := s.buildClient("beer.BeerCellarService")

	if err != nil {
		t.Errorf("Error in building client: %v,%v", err, client)
	}
}

func InitTest() *Server {
	s := Init()
	s.dialler = &testDialler{}
	return s
}

type testDialler struct {
	fail bool
}

func (t *testDialler) dial() (*grpc.ClientConn, error) {
	if t.fail {
		return nil, fmt.Errorf("Built to fail")
	}
	return grpc.Dial("192.168.86.1:50000", grpc.WithInsecure())
}

type testBeerServer struct {
	callList int
	fail     bool
}

func (t *testBeerServer) ListBeers(ctx context.Context, req *bspb.ListBeerRequest, opts ...grpc.CallOption) (*bspb.ListBeerResponse, error) {
	t.callList++
	if t.fail {
		return nil, fmt.Errorf("Built to fial")
	}
	return &bspb.ListBeerResponse{}, nil
}

func (t *testBeerServer) AddBeer(ctx context.Context, in *bspb.AddBeerRequest, opts ...grpc.CallOption) (*bspb.AddBeerResponse, error) {
	return nil, nil
}

func (t *testBeerServer) DeleteBeer(ctx context.Context, in *bspb.DeleteBeerRequest, opts ...grpc.CallOption) (*bspb.DeleteBeerResponse, error) {
	return nil, nil
}

func (t *testBeerServer) NewTestBeerServer(conn *grpc.ClientConn) bspb.BeerCellarServiceClient {
	return t
}

func TestHandleFailClient(t *testing.T) {
	s := InitTest()
	tbs := &testBeerServer{}
	s.add("beer.BeerCellarService", tbs.NewTestBeerServer)

	val, err := s.handle(context.Background(), "beer.BeerCellarServiceDOESNOTEXIST", "ListBeers", "{\"onDeck\": true}")
	if err == nil {
		t.Errorf("Bad parse: %v -> %v", val, err)
	}
}

func TestHandleFailDial(t *testing.T) {
	s := InitTest()
	s.dialler = &testDialler{fail: true}
	tbs := &testBeerServer{}
	s.add("beer.BeerCellarService", tbs.NewTestBeerServer)

	val, err := s.handle(context.Background(), "beer.BeerCellarService", "ListBeers", "{\"onDeck\": true}")
	if err == nil {
		t.Errorf("Bad parse: %v -> %v", val, err)
	}
}

func TestHandleFailCall(t *testing.T) {
	s := InitTest()
	tbs := &testBeerServer{fail: true}
	s.add("beer.BeerCellarService", tbs.NewTestBeerServer)

	val, err := s.handle(context.Background(), "beer.BeerCellarService", "ListBeers", "{\"onDeck\": true}")
	if err == nil {
		t.Errorf("Bad parse: %v -> %v", val, err)
	}
}

func TestHandleFailBuildReq(t *testing.T) {
	s := InitTest()
	tbs := &testBeerServer{fail: true}
	s.add("beer.BeerCellarService", tbs.NewTestBeerServer)

	val, err := s.handle(context.Background(), "beer.BeerCellarService", "ListBeers", "{\"onMADEUPDeck\": true}")
	if err == nil {
		t.Errorf("Bad parse: %v -> %v", val, err)
	}
}

func TestHandle(t *testing.T) {
	s := InitTest()
	tbs := &testBeerServer{}
	s.add("beer.BeerCellarService", tbs.NewTestBeerServer)

	val, err := s.handle(context.Background(), "beer.BeerCellarService", "ListBeers", "{\"onDeck\": true}")
	if err != nil {
		t.Errorf("Bad parse: %v -> %v", val, err)
	}

	if tbs.callList != 1 {
		t.Errorf("TBS was not called")
	}
}

func TestBuildData(t *testing.T) {
	req := &bspb.ListBeerRequest{OnDeck: true}
	marshaler := &jsonpb.Marshaler{}
	str, err := marshaler.MarshalToString(req)

	if err != nil {
		t.Errorf("Bad marshal: %v", err)
	}

	log.Printf("Marshal to: %v from %v", str, req)

	s := InitTest()
	data, err := s.buildRequest("beer.ListBeerRequest", []byte(str))

	if err != nil {
		t.Errorf("Bad Build: %v", err)
	}

	conv, ok := data.(*bspb.ListBeerRequest)
	if !ok {
		t.Errorf("Conversion issue: %+v", conv)
	}

	if !conv.GetOnDeck() {
		t.Errorf("Bad conversion: %v", conv)
	}
}
