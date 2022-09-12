package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"github.com/brotherlogic/goserver/utils"
	lpb "github.com/brotherlogic/login/proto"

	_ "google.golang.org/grpc/encoding/gzip"
)

var (
	clientStreamDescForProxying = &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}
)

type handler struct {
	passes  map[string]int
	log     func(context.Context, string)
	dialOut func(ctx context.Context, server string) (*grpc.ClientConn, error)
	dial    func(host string, opts ...grpc.DialOption) (*grpc.ClientConn, error)
}

func (s *handler) authorize(ctx context.Context, auth string) error {
	conn, err := s.dialOut(ctx, "login")
	if err != nil {
		return err
	}
	defer conn.Close()

	client := lpb.NewLoginServiceClient(conn)
	_, err = client.Authenticate(ctx, &lpb.AuthenticateRequest{Token: auth})
	return err
}

func getCtx(ctx context.Context) context.Context {
	md, _ := metadata.FromIncomingContext(ctx)
	outCtx, _ := context.WithCancel(ctx)
	outCtx = metadata.NewOutgoingContext(outCtx, md.Copy())
	return outCtx
}

func (s *handler) handler(srv interface{}, serverStream grpc.ServerStream) error {
	ctx, cancel := utils.ManualContext("sp-handler", time.Minute)
	defer cancel()
	fullMethodName, ok := grpc.MethodFromServerStream(serverStream)
	if !ok {
		return fmt.Errorf("Bad name")
	}
	s.passes[fullMethodName]++
	parts := strings.Split(fullMethodName[1:], ".")
	outgoingCtx := getCtx(serverStream.Context())

	auth := ""
	md, ok := metadata.FromIncomingContext(outgoingCtx)
	if ok {
		authHeaders, ok := md["auth"]
		if ok {
			if len(authHeaders) == 1 {
				auth = authHeaders[0]
			} else {
				s.log(ctx, fmt.Sprintf("WEIRD %v", authHeaders))
			}

		}
	}

	s.log(ctx, fmt.Sprintf("Handling %v with %v, %v", fullMethodName, outgoingCtx, auth))
	if fullMethodName != "/login.LoginService/Login" {
		if err := s.authorize(outgoingCtx, auth); err != nil {
			return fmt.Errorf("%v is an unauthorized request: %v", fullMethodName, err)
		}
	}

	backendConn, err := s.dial(parts[0], grpc.WithCodec(Codec()))
	if err != nil {
		return err
	}

	clientCtx, clientCancel := context.WithCancel(outgoingCtx)
	clientStream, err := grpc.NewClientStream(clientCtx, clientStreamDescForProxying, backendConn, fullMethodName)
	if err != nil {
		return err
	}

	s2cErrChan := s.forwardServerToClient(serverStream, clientStream)
	c2sErrChan := s.forwardClientToServer(clientStream, serverStream)

	for i := 0; i < 2; i++ {
		select {
		case s2cErr := <-s2cErrChan:
			if s2cErr == io.EOF {
				clientStream.CloseSend()
				break
			} else {
				clientCancel()
				return grpc.Errorf(codes.Internal, "failed proxying s2c: %v", s2cErr)
			}
		case c2sErr := <-c2sErrChan:
			serverStream.SetTrailer(clientStream.Trailer())

			if c2sErr != io.EOF {
				return c2sErr
			}
			return nil
		}
	}
	return grpc.Errorf(codes.Internal, "gRPC proxying should never reach this stage.")
}

func (s *handler) forwardClientToServer(src grpc.ClientStream, dst grpc.ServerStream) chan error {
	ret := make(chan error, 1)
	go func() {
		f := &frame{}
		for i := 0; ; i++ {
			if err := src.RecvMsg(f); err != nil {
				ret <- err
				break
			}
			if i == 0 {
				md, err := src.Header()
				if err != nil {
					ret <- err
					break
				}
				if err := dst.SendHeader(md); err != nil {
					ret <- err
					break
				}
			}
			if err := dst.SendMsg(f); err != nil {
				ret <- err
				break
			}
		}
	}()
	return ret
}

func (s *handler) forwardServerToClient(src grpc.ServerStream, dst grpc.ClientStream) chan error {
	ret := make(chan error, 1)
	go func() {
		f := &frame{}
		for i := 0; ; i++ {
			if err := src.RecvMsg(f); err != nil {
				ret <- err
				break
			}
			if err := dst.SendMsg(f); err != nil {
				ret <- err
				break
			}
		}
	}()
	return ret
}

func valid(auth []string) bool {
	return false
}

func unaryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	// authentication (token verification)
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, fmt.Errorf("No metadata")
	}
	if !valid(md["authorization"]) {
		return nil, fmt.Errorf("Invalid token")
	}
	return handler(ctx, req)
}
