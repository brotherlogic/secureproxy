package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/brotherlogic/goserver"
	"github.com/brotherlogic/goserver/utils"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	bspb "github.com/brotherlogic/beerserver/proto"
	dpb "github.com/brotherlogic/dashboard/proto"
	pbg "github.com/brotherlogic/goserver/proto"
	lpb "github.com/brotherlogic/login/proto"
)

// Server main server type
type Server struct {
	*goserver.GoServer
	handler handler
	cmap    map[string]interface{}
	dialler dialler
}

// Init builds the server
func Init() *Server {
	s := &Server{
		GoServer: &goserver.GoServer{},
		cmap:     make(map[string]interface{}),
	}
	s.dialler = &prodDialler{hdial: s.FDial}

	s.buildClients()

	return s
}

func (s *Server) buildClients() {
	s.cmap["beerserver.BeerCellarService"] = bspb.NewBeerCellarServiceClient
	s.cmap["login.LoginService"] = lpb.NewLoginServiceClient
	s.cmap["dashboard.DashboardService"] = dpb.NewDashboardServiceClient
}

func (s *Server) add(key string, val interface{}) {
	s.cmap[key] = val
}

// DoRegister does RPC registration
func (s *Server) DoRegister(server *grpc.Server) {
	//pass
}

// ReportHealth alerts if we're not healthy
func (s *Server) ReportHealth() bool {
	return true
}

// Shutdown the server
func (s *Server) Shutdown(ctx context.Context) error {
	return nil
}

// Mote promotes/demotes this server
func (s *Server) Mote(ctx context.Context, master bool) error {
	return nil
}

// GetState gets the state of the server
func (s *Server) GetState() []*pbg.State {
	ret := []*pbg.State{}
	for key, count := range s.handler.passes {
		ret = append(ret, &pbg.State{Key: key, Value: int64(count)})
	}

	sort.SliceStable(ret, func(i, j int) bool {
		return ret[i].Key < ret[j].Key
	})
	return ret
}

func (s *Server) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	ctx, cancel := utils.ManualContext("secureproxy", time.Minute*5)
	defer cancel()

	resp.Header().Set("Access-Control-Allow-Origin", "*")
	resp.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
	resp.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

	s.CtxLog(ctx, fmt.Sprintf("Handling Request: %v with %v -> %+v", req.URL.Path, req.Header.Get("auth"), req.Header))
	time.Sleep(time.Second * 2)
	// Remove the / prefix
	parts := strings.Split(req.URL.Path[1:], "/")

	// Straight through return (200 OK)
	if len(parts) != 2 || req.Method != "POST" {
		s.CtxLog(ctx, fmt.Sprintf("PARTS %v %v", len(parts), req.Method))
		return
	}

	defer req.Body.Close()
	bodyd, err := ioutil.ReadAll(req.Body)
	if err != nil {
		s.CtxLog(ctx, fmt.Sprintf("Cannot read body: %v", err))
	}

	service := parts[0]
	method := parts[1]

	res, err := s.handle(ctx, service, method, string(bodyd))

	if err != nil {
		s.CtxLog(ctx, fmt.Sprintf("Error in handler: %v", err))
		resp.WriteHeader(501)
		resp.Write([]byte(fmt.Sprintf("%v", err)))
		return
	}

	resp.Write([]byte(res))
}

func (s *Server) serveUp(port int) error {
	return http.ListenAndServe(fmt.Sprintf(":%v", port), s)
}

type dialler interface {
	dial(ctx context.Context) (*grpc.ClientConn, error)
}

type prodDialler struct {
	hdial func(server string) (*grpc.ClientConn, error)
}

func (p *prodDialler) dial(ctx context.Context) (*grpc.ClientConn, error) {
	return p.hdial("localhost:50040")
}

func main() {
	var quiet = flag.Bool("quiet", false, "Show all output")
	flag.Parse()

	//Turn off logging
	if *quiet {
		log.SetFlags(0)
		log.SetOutput(ioutil.Discard)
	}
	server := Init()
	server.PrepServer("secureproxy")
	server.Register = server

	err := server.RegisterServerV2(true)
	if err != nil {
		return
	}

	go func() {
		err := server.serveUp(int(server.Registry.Port - 1))
		if err != nil {
			log.Fatalf("Unable to serve http traffic: %v", err)
		}
	}()

	server.handler = handler{passes: make(map[string]int), log: server.CtxLog, dialOut: server.FDialServer}
	err = server.Serve(grpc.CustomCodec(Codec()), grpc.UnknownServiceHandler(server.handler.handler))
	if err != nil {
		fmt.Printf("%v\n", err)
	}
}
