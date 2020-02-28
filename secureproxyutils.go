package main

import "fmt"

func (s *Server) handle(service, method, data string) (string, error) {
	s.Log(fmt.Sprintf("Handling %v/%v with %v", service, method, data))
	return "", nil
}
