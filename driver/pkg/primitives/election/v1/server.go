// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	api "github.com/atomix/multi-raft-storage/api/atomix/multiraft/election/v1"
	multiraftv1 "github.com/atomix/multi-raft-storage/api/atomix/multiraft/v1"
	"github.com/atomix/multi-raft-storage/driver/pkg/client"
	"github.com/atomix/multi-raft-storage/driver/pkg/util/async"
	electionv1 "github.com/atomix/runtime/api/atomix/runtime/election/v1"
	"github.com/atomix/runtime/sdk/pkg/errors"
	"github.com/atomix/runtime/sdk/pkg/logging"
	"github.com/atomix/runtime/sdk/pkg/runtime"
	"github.com/atomix/runtime/sdk/pkg/stringer"
	"google.golang.org/grpc"
	"io"
)

var log = logging.GetLogger()

const Service = "atomix.runtime.election.v1.LeaderElection"

const truncLen = 200

func NewLeaderElectionServer(protocol *client.Protocol, spec runtime.PrimitiveSpec) (electionv1.LeaderElectionServer, error) {
	return &multiRaftLeaderElectionServer{
		Protocol:      protocol,
		PrimitiveSpec: spec,
	}, nil
}

type multiRaftLeaderElectionServer struct {
	*client.Protocol
	runtime.PrimitiveSpec
}

func (s *multiRaftLeaderElectionServer) Create(ctx context.Context, request *electionv1.CreateRequest) (*electionv1.CreateResponse, error) {
	log.Debugw("Create",
		logging.Stringer("CreateRequest", stringer.Truncate(request, truncLen)))
	partitions := s.Partitions()
	err := async.IterAsync(len(partitions), func(i int) error {
		partition := partitions[i]
		session, err := partition.GetSession(ctx)
		if err != nil {
			return err
		}
		return session.CreatePrimitive(ctx, s.PrimitiveSpec)
	})
	if err != nil {
		log.Warnw("Create",
			logging.Stringer("CreateRequest", stringer.Truncate(request, truncLen)),
			logging.Error("Error", err))
		return nil, errors.ToProto(err)
	}
	response := &electionv1.CreateResponse{}
	log.Debugw("Create",
		logging.Stringer("CreateRequest", stringer.Truncate(request, truncLen)),
		logging.Stringer("CreateResponse", stringer.Truncate(response, truncLen)))
	return response, nil
}

func (s *multiRaftLeaderElectionServer) Close(ctx context.Context, request *electionv1.CloseRequest) (*electionv1.CloseResponse, error) {
	log.Debugw("Close",
		logging.Stringer("CloseRequest", stringer.Truncate(request, truncLen)))
	partitions := s.Partitions()
	err := async.IterAsync(len(partitions), func(i int) error {
		partition := partitions[i]
		session, err := partition.GetSession(ctx)
		if err != nil {
			return err
		}
		return session.ClosePrimitive(ctx, request.ID.Name)
	})
	if err != nil {
		log.Warnw("Close",
			logging.Stringer("CloseRequest", stringer.Truncate(request, truncLen)),
			logging.Error("Error", err))
		return nil, errors.ToProto(err)
	}
	response := &electionv1.CloseResponse{}
	log.Debugw("Close",
		logging.Stringer("CloseRequest", stringer.Truncate(request, truncLen)),
		logging.Stringer("CloseResponse", stringer.Truncate(response, truncLen)))
	return response, nil
}

func (s *multiRaftLeaderElectionServer) Enter(ctx context.Context, request *electionv1.EnterRequest) (*electionv1.EnterResponse, error) {
	log.Debugw("Enter",
		logging.Stringer("EnterRequest", stringer.Truncate(request, truncLen)))
	partition := s.PartitionBy([]byte(request.ID.Name))
	session, err := partition.GetSession(ctx)
	if err != nil {
		log.Warnw("Enter",
			logging.Stringer("EnterRequest", stringer.Truncate(request, truncLen)),
			logging.Error("Error", err))
		return nil, errors.ToProto(err)
	}
	primitive, err := session.GetPrimitive(request.ID.Name)
	if err != nil {
		log.Warnw("Enter",
			logging.Stringer("EnterRequest", stringer.Truncate(request, truncLen)),
			logging.Error("Error", err))
		return nil, errors.ToProto(err)
	}
	command := client.Command[*api.EnterResponse](primitive)
	output, err := command.Run(func(conn *grpc.ClientConn, headers *multiraftv1.CommandRequestHeaders) (*api.EnterResponse, error) {
		return api.NewLeaderElectionClient(conn).Enter(ctx, &api.EnterRequest{
			Headers: headers,
			EnterInput: &api.EnterInput{
				Candidate: request.Candidate,
			},
		})
	})
	if err != nil {
		log.Warnw("Enter",
			logging.Stringer("EnterRequest", stringer.Truncate(request, truncLen)),
			logging.Error("Error", err))
		return nil, errors.ToProto(err)
	}
	response := &electionv1.EnterResponse{
		Term: electionv1.Term{
			Term:       uint64(output.Term.Index),
			Leader:     output.Term.Leader,
			Candidates: output.Term.Candidates,
		},
	}
	log.Debugw("Enter",
		logging.Stringer("EnterRequest", stringer.Truncate(request, truncLen)),
		logging.Stringer("EnterResponse", stringer.Truncate(response, truncLen)))
	return response, nil
}

func (s *multiRaftLeaderElectionServer) Withdraw(ctx context.Context, request *electionv1.WithdrawRequest) (*electionv1.WithdrawResponse, error) {
	log.Debugw("Withdraw",
		logging.Stringer("WithdrawRequest", stringer.Truncate(request, truncLen)))
	partition := s.PartitionBy([]byte(request.ID.Name))
	session, err := partition.GetSession(ctx)
	if err != nil {
		log.Warnw("Withdraw",
			logging.Stringer("WithdrawRequest", stringer.Truncate(request, truncLen)),
			logging.Error("Error", err))
		return nil, errors.ToProto(err)
	}
	primitive, err := session.GetPrimitive(request.ID.Name)
	if err != nil {
		log.Warnw("Withdraw",
			logging.Stringer("WithdrawRequest", stringer.Truncate(request, truncLen)),
			logging.Error("Error", err))
		return nil, errors.ToProto(err)
	}
	command := client.Command[*api.WithdrawResponse](primitive)
	output, err := command.Run(func(conn *grpc.ClientConn, headers *multiraftv1.CommandRequestHeaders) (*api.WithdrawResponse, error) {
		return api.NewLeaderElectionClient(conn).Withdraw(ctx, &api.WithdrawRequest{
			Headers: headers,
			WithdrawInput: &api.WithdrawInput{
				Candidate: request.Candidate,
			},
		})
	})
	if err != nil {
		log.Warnw("Withdraw",
			logging.Stringer("WithdrawRequest", stringer.Truncate(request, truncLen)),
			logging.Error("Error", err))
		return nil, errors.ToProto(err)
	}
	response := &electionv1.WithdrawResponse{
		Term: electionv1.Term{
			Term:       uint64(output.Term.Index),
			Leader:     output.Term.Leader,
			Candidates: output.Term.Candidates,
		},
	}
	log.Debugw("Withdraw",
		logging.Stringer("WithdrawRequest", stringer.Truncate(request, truncLen)),
		logging.Stringer("WithdrawResponse", stringer.Truncate(response, truncLen)))
	return response, nil
}

func (s *multiRaftLeaderElectionServer) Anoint(ctx context.Context, request *electionv1.AnointRequest) (*electionv1.AnointResponse, error) {
	log.Debugw("Anoint",
		logging.Stringer("AnointRequest", stringer.Truncate(request, truncLen)))
	partition := s.PartitionBy([]byte(request.ID.Name))
	session, err := partition.GetSession(ctx)
	if err != nil {
		log.Warnw("Anoint",
			logging.Stringer("AnointRequest", stringer.Truncate(request, truncLen)),
			logging.Error("Error", err))
		return nil, errors.ToProto(err)
	}
	primitive, err := session.GetPrimitive(request.ID.Name)
	if err != nil {
		log.Warnw("Anoint",
			logging.Stringer("AnointRequest", stringer.Truncate(request, truncLen)),
			logging.Error("Error", err))
		return nil, errors.ToProto(err)
	}
	command := client.Command[*api.AnointResponse](primitive)
	output, err := command.Run(func(conn *grpc.ClientConn, headers *multiraftv1.CommandRequestHeaders) (*api.AnointResponse, error) {
		return api.NewLeaderElectionClient(conn).Anoint(ctx, &api.AnointRequest{
			Headers: headers,
			AnointInput: &api.AnointInput{
				Candidate: request.Candidate,
			},
		})
	})
	if err != nil {
		log.Warnw("Anoint",
			logging.Stringer("AnointRequest", stringer.Truncate(request, truncLen)),
			logging.Error("Error", err))
		return nil, errors.ToProto(err)
	}
	response := &electionv1.AnointResponse{
		Term: electionv1.Term{
			Term:       uint64(output.Term.Index),
			Leader:     output.Term.Leader,
			Candidates: output.Term.Candidates,
		},
	}
	log.Debugw("Anoint",
		logging.Stringer("AnointRequest", stringer.Truncate(request, truncLen)),
		logging.Stringer("AnointResponse", stringer.Truncate(response, truncLen)))
	return response, nil
}

func (s *multiRaftLeaderElectionServer) Promote(ctx context.Context, request *electionv1.PromoteRequest) (*electionv1.PromoteResponse, error) {
	log.Debugw("Promote",
		logging.Stringer("PromoteRequest", stringer.Truncate(request, truncLen)))
	partition := s.PartitionBy([]byte(request.ID.Name))
	session, err := partition.GetSession(ctx)
	if err != nil {
		log.Warnw("Promote",
			logging.Stringer("PromoteRequest", stringer.Truncate(request, truncLen)),
			logging.Error("Error", err))
		return nil, errors.ToProto(err)
	}
	primitive, err := session.GetPrimitive(request.ID.Name)
	if err != nil {
		log.Warnw("Promote",
			logging.Stringer("PromoteRequest", stringer.Truncate(request, truncLen)),
			logging.Error("Error", err))
		return nil, errors.ToProto(err)
	}
	query := client.Command[*api.PromoteResponse](primitive)
	output, err := query.Run(func(conn *grpc.ClientConn, headers *multiraftv1.CommandRequestHeaders) (*api.PromoteResponse, error) {
		return api.NewLeaderElectionClient(conn).Promote(ctx, &api.PromoteRequest{
			Headers: headers,
			PromoteInput: &api.PromoteInput{
				Candidate: request.Candidate,
			},
		})
	})
	if err != nil {
		log.Warnw("Promote",
			logging.Stringer("PromoteRequest", stringer.Truncate(request, truncLen)),
			logging.Error("Error", err))
		return nil, errors.ToProto(err)
	}
	response := &electionv1.PromoteResponse{
		Term: electionv1.Term{
			Term:       uint64(output.Term.Index),
			Leader:     output.Term.Leader,
			Candidates: output.Term.Candidates,
		},
	}
	log.Debugw("Promote",
		logging.Stringer("PromoteRequest", stringer.Truncate(request, truncLen)),
		logging.Stringer("PromoteResponse", stringer.Truncate(response, truncLen)))
	return response, nil
}

func (s *multiRaftLeaderElectionServer) Demote(ctx context.Context, request *electionv1.DemoteRequest) (*electionv1.DemoteResponse, error) {
	log.Debugw("Demote",
		logging.Stringer("DemoteRequest", stringer.Truncate(request, truncLen)))
	partition := s.PartitionBy([]byte(request.ID.Name))
	session, err := partition.GetSession(ctx)
	if err != nil {
		log.Warnw("Demote",
			logging.Stringer("DemoteRequest", stringer.Truncate(request, truncLen)),
			logging.Error("Error", err))
		return nil, errors.ToProto(err)
	}
	primitive, err := session.GetPrimitive(request.ID.Name)
	if err != nil {
		log.Warnw("Demote",
			logging.Stringer("DemoteRequest", stringer.Truncate(request, truncLen)),
			logging.Error("Error", err))
		return nil, errors.ToProto(err)
	}
	query := client.Command[*api.DemoteResponse](primitive)
	output, err := query.Run(func(conn *grpc.ClientConn, headers *multiraftv1.CommandRequestHeaders) (*api.DemoteResponse, error) {
		return api.NewLeaderElectionClient(conn).Demote(ctx, &api.DemoteRequest{
			Headers: headers,
			DemoteInput: &api.DemoteInput{
				Candidate: request.Candidate,
			},
		})
	})
	if err != nil {
		log.Warnw("Demote",
			logging.Stringer("DemoteRequest", stringer.Truncate(request, truncLen)),
			logging.Error("Error", err))
		return nil, errors.ToProto(err)
	}
	response := &electionv1.DemoteResponse{
		Term: electionv1.Term{
			Term:       uint64(output.Term.Index),
			Leader:     output.Term.Leader,
			Candidates: output.Term.Candidates,
		},
	}
	log.Debugw("Demote",
		logging.Stringer("DemoteRequest", stringer.Truncate(request, truncLen)),
		logging.Stringer("DemoteResponse", stringer.Truncate(response, truncLen)))
	return response, nil
}

func (s *multiRaftLeaderElectionServer) Evict(ctx context.Context, request *electionv1.EvictRequest) (*electionv1.EvictResponse, error) {
	log.Debugw("Evict",
		logging.Stringer("EvictRequest", stringer.Truncate(request, truncLen)))
	partition := s.PartitionBy([]byte(request.ID.Name))
	session, err := partition.GetSession(ctx)
	if err != nil {
		log.Warnw("Evict",
			logging.Stringer("EvictRequest", stringer.Truncate(request, truncLen)),
			logging.Error("Error", err))
		return nil, errors.ToProto(err)
	}
	primitive, err := session.GetPrimitive(request.ID.Name)
	if err != nil {
		log.Warnw("Evict",
			logging.Stringer("EvictRequest", stringer.Truncate(request, truncLen)),
			logging.Error("Error", err))
		return nil, errors.ToProto(err)
	}
	command := client.Command[*api.EvictResponse](primitive)
	output, err := command.Run(func(conn *grpc.ClientConn, headers *multiraftv1.CommandRequestHeaders) (*api.EvictResponse, error) {
		return api.NewLeaderElectionClient(conn).Evict(ctx, &api.EvictRequest{
			Headers: headers,
			EvictInput: &api.EvictInput{
				Candidate: request.Candidate,
			},
		})
	})
	if err != nil {
		log.Warnw("Evict",
			logging.Stringer("EvictRequest", stringer.Truncate(request, truncLen)),
			logging.Error("Error", err))
		return nil, errors.ToProto(err)
	}
	response := &electionv1.EvictResponse{
		Term: electionv1.Term{
			Term:       uint64(output.Term.Index),
			Leader:     output.Term.Leader,
			Candidates: output.Term.Candidates,
		},
	}
	log.Debugw("Evict",
		logging.Stringer("EvictRequest", stringer.Truncate(request, truncLen)),
		logging.Stringer("EvictResponse", stringer.Truncate(response, truncLen)))
	return response, nil
}

func (s *multiRaftLeaderElectionServer) GetTerm(ctx context.Context, request *electionv1.GetTermRequest) (*electionv1.GetTermResponse, error) {
	log.Debugw("GetTerm",
		logging.Stringer("GetTermRequest", stringer.Truncate(request, truncLen)))
	partition := s.PartitionBy([]byte(request.ID.Name))
	session, err := partition.GetSession(ctx)
	if err != nil {
		log.Warnw("GetTerm",
			logging.Stringer("GetTermRequest", stringer.Truncate(request, truncLen)),
			logging.Error("Error", err))
		return nil, errors.ToProto(err)
	}
	primitive, err := session.GetPrimitive(request.ID.Name)
	if err != nil {
		log.Warnw("GetTerm",
			logging.Stringer("GetTermRequest", stringer.Truncate(request, truncLen)),
			logging.Error("Error", err))
		return nil, errors.ToProto(err)
	}
	query := client.Query[*api.GetTermResponse](primitive)
	output, err := query.Run(func(conn *grpc.ClientConn, headers *multiraftv1.QueryRequestHeaders) (*api.GetTermResponse, error) {
		return api.NewLeaderElectionClient(conn).GetTerm(ctx, &api.GetTermRequest{
			Headers:      headers,
			GetTermInput: &api.GetTermInput{},
		})
	})
	if err != nil {
		log.Warnw("GetTerm",
			logging.Stringer("GetTermRequest", stringer.Truncate(request, truncLen)),
			logging.Error("Error", err))
		return nil, errors.ToProto(err)
	}
	response := &electionv1.GetTermResponse{
		Term: electionv1.Term{
			Term:       uint64(output.Term.Index),
			Leader:     output.Term.Leader,
			Candidates: output.Term.Candidates,
		},
	}
	log.Debugw("GetTerm",
		logging.Stringer("GetTermRequest", stringer.Truncate(request, truncLen)),
		logging.Stringer("GetTermResponse", stringer.Truncate(response, truncLen)))
	return response, nil
}

func (s *multiRaftLeaderElectionServer) Watch(request *electionv1.WatchRequest, server electionv1.LeaderElection_WatchServer) error {
	log.Debugw("Watch",
		logging.Stringer("WatchRequest", stringer.Truncate(request, truncLen)))
	partition := s.PartitionBy([]byte(request.ID.Name))
	session, err := partition.GetSession(server.Context())
	if err != nil {
		log.Warnw("Watch",
			logging.Stringer("WatchRequest", stringer.Truncate(request, truncLen)),
			logging.Error("Error", err))
		return errors.ToProto(err)
	}
	primitive, err := session.GetPrimitive(request.ID.Name)
	if err != nil {
		log.Warnw("Watch",
			logging.Stringer("WatchRequest", stringer.Truncate(request, truncLen)),
			logging.Error("Error", err))
		return errors.ToProto(err)
	}
	query := client.StreamQuery[*api.WatchResponse](primitive)
	stream, err := query.Run(func(conn *grpc.ClientConn, headers *multiraftv1.QueryRequestHeaders) (client.QueryStream[*api.WatchResponse], error) {
		return api.NewLeaderElectionClient(conn).Watch(server.Context(), &api.WatchRequest{
			Headers:    headers,
			WatchInput: &api.WatchInput{},
		})
	})
	if err != nil {
		log.Warnw("Watch",
			logging.Stringer("WatchRequest", stringer.Truncate(request, truncLen)),
			logging.Error("Error", err))
		return errors.ToProto(err)
	}
	for {
		output, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			log.Warnw("Watch",
				logging.Stringer("WatchRequest", stringer.Truncate(request, truncLen)),
				logging.Error("Error", err))
			return errors.ToProto(err)
		}
		response := &electionv1.WatchResponse{
			Term: electionv1.Term{
				Term:       uint64(output.Term.Index),
				Leader:     output.Term.Leader,
				Candidates: output.Term.Candidates,
			},
		}
		log.Debugw("Watch",
			logging.Stringer("WatchRequest", stringer.Truncate(request, truncLen)),
			logging.Stringer("WatchResponse", stringer.Truncate(response, truncLen)))
		if err := server.Send(response); err != nil {
			log.Warnw("Watch",
				logging.Stringer("WatchRequest", stringer.Truncate(request, truncLen)),
				logging.Stringer("WatchResponse", stringer.Truncate(response, truncLen)),
				logging.Error("Error", err))
			return err
		}
	}
}

var _ electionv1.LeaderElectionServer = (*multiRaftLeaderElectionServer)(nil)
