package transport

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

type Server struct{}

func (s *Server) Register() error {
	return grpc.Errorf(codes.Unimplemented, "Currently Unimplemented")
}
