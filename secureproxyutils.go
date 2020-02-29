package main

import (
	"bytes"
	"context"
	"fmt"
	"reflect"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
)

func (s *Server) handle(ctx context.Context, service, method, data string) (string, error) {
	s.Log(fmt.Sprintf("Handling %v/%v with %v", service, method, data))

	clientf, err := s.buildClient(service)
	if err != nil {
		return "", err
	}

	conn, err := s.dialler.dial()
	if err != nil {
		return "", err
	}
	defer conn.Close()

	fc := reflect.ValueOf(clientf)
	pieces := fc.Call([]reflect.Value{reflect.ValueOf(conn)})
	methodFunc := reflect.ValueOf(pieces[0].Interface()).MethodByName(method).Interface()

	fType := reflect.TypeOf(methodFunc)

	//Drop the pointer * from the type name
	reqName := fType.In(1).String()[1:]

	req, err := s.buildRequest(reqName, []byte(data))
	if err != nil {
		return "", err
	}

	results := reflect.ValueOf(methodFunc).Call([]reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(req)})

	err = nil
	if results[1].Interface() != nil {
		return "", results[1].Interface().(error)

	}

	// We're assuming that we got here, and we got a proto response. This should not fail.
	marshaler := &jsonpb.Marshaler{}
	str, _ := marshaler.MarshalToString(results[0].Interface().(proto.Message))

	return str, err
}

func (s *Server) buildClient(service string) (interface{}, error) {
	if val, ok := s.cmap[service]; ok {
		return val, nil
	}
	return nil, fmt.Errorf("Not quite implemented")
}

func (s *Server) buildRequest(name string, data []byte) (interface{}, error) {
	val := proto.MessageType(name)

	req := reflect.New(val.Elem()).Interface().(proto.Message)

	err := jsonpb.Unmarshal(bytes.NewReader(data), req)

	return req, err
}
