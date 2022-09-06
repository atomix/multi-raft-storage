// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package primitive

import (
	statemachine "github.com/atomix/multi-raft-storage/node/pkg/statemachine2"
	"github.com/atomix/multi-raft-storage/node/pkg/statemachine2/session"
	"github.com/atomix/multi-raft-storage/node/pkg/statemachine2/snapshot"
	"github.com/atomix/runtime/sdk/pkg/logging"
	"github.com/gogo/protobuf/proto"
)

type NewPrimitiveFunc[I, O any] func(Context[I, O]) Primitive[I, O]

type Primitive[I, O any] interface {
	snapshot.Recoverable
	Propose(proposal session.Proposal[I, O])
	Query(query session.Query[I, O])
}

type Type[I, O any] interface {
	Service() string
	Codec() Codec[I, O]
	NewStateMachine(Context[I, O]) Primitive[I, O]
}

func NewType[I, O any](service string, codec Codec[I, O], factory NewPrimitiveFunc[I, O]) Type[I, O] {
	return &primitiveType[I, O]{
		service: service,
		codec:   codec,
		factory: factory,
	}
}

type primitiveType[I, O any] struct {
	service string
	codec   Codec[I, O]
	factory func(Context[I, O]) Primitive[I, O]
}

func (t *primitiveType[I, O]) Service() string {
	return t.service
}

func (t *primitiveType[I, O]) Codec() Codec[I, O] {
	return t.codec
}

func (t *primitiveType[I, O]) NewStateMachine(context Context[I, O]) Primitive[I, O] {
	return t.factory(context)
}

type ID uint64

type Context[I, O any] interface {
	session.Context[I, O]
	// ID returns the service identifier
	ID() ID
	// Service returns the service name
	Service() string
	// Namespace returns the service namespace
	Namespace() string
	// Name returns the service name
	Name() string
}

type Executor[T session.Execution[U, I, O], U statemachine.ExecutionID, I, O any] interface {
	Execute(T)
}

type Proposer[I1, O1, I2, O2 any] interface {
	Executor[session.Proposal[I1, O1], statemachine.ProposalID, I1, O1]
	Proposals() session.Proposals[I2, O2]
}

func NewProposer[I1, O1, I2, O2 proto.Message](ctx Context[I1, O1]) *ProposerBuilder[I1, O1, I2, O2] {
	return &ProposerBuilder[I1, O1, I2, O2]{
		ctx: ctx,
	}
}

type ProposerBuilder[I1, O1, I2, O2 proto.Message] struct {
	ctx     Context[I1, O1]
	name    string
	decoder func(I1) (I2, bool)
	encoder func(O2) O1
}

func (b *ProposerBuilder[I1, O1, I2, O2]) Name(name string) *ProposerBuilder[I1, O1, I2, O2] {
	b.name = name
	return b
}

func (b *ProposerBuilder[I1, O1, I2, O2]) Decoder(f func(I1) (I2, bool)) *ProposerBuilder[I1, O1, I2, O2] {
	b.decoder = f
	return b
}

func (b *ProposerBuilder[I1, O1, I2, O2]) Encoder(f func(O2) O1) *ProposerBuilder[I1, O1, I2, O2] {
	b.encoder = f
	return b
}

func (b *ProposerBuilder[I1, O1, I2, O2]) Build(f func(session.Proposal[I2, O2])) Proposer[I1, O1, I2, O2] {
	return &transcodingProposer[I1, O1, I2, O2]{
		ctx:     b.ctx,
		decoder: b.decoder,
		encoder: b.encoder,
		name:    b.name,
		f:       f,
	}
}

var _ ExecutorBuilder[
	session.Proposal[proto.Message, proto.Message],
	statemachine.ProposalID,
	proto.Message,
	proto.Message,
	Proposer[proto.Message, proto.Message, proto.Message, proto.Message]] = (*ProposerBuilder[proto.Message, proto.Message, proto.Message, proto.Message])(nil)

type ExecutorBuilder[
	T session.Execution[U, I, O],
	U statemachine.ExecutionID,
	I proto.Message,
	O proto.Message,
	E Executor[T, U, I, O]] interface {
	Build(f func(T)) E
}

type Querier[I1, O1, I2, O2 any] interface {
	Executor[session.Query[I1, O1], statemachine.QueryID, I1, O1]
}

func NewQuerier[I1, O1, I2, O2 proto.Message](ctx Context[I1, O1]) *QuerierBuilder[I1, O1, I2, O2] {
	return &QuerierBuilder[I1, O1, I2, O2]{
		ctx: ctx,
	}
}

type QuerierBuilder[I1, O1, I2, O2 proto.Message] struct {
	ctx     Context[I1, O1]
	name    string
	decoder func(I1) (I2, bool)
	encoder func(O2) O1
}

func (b *QuerierBuilder[I1, O1, I2, O2]) Name(name string) *QuerierBuilder[I1, O1, I2, O2] {
	b.name = name
	return b
}

func (b *QuerierBuilder[I1, O1, I2, O2]) Decoder(f func(I1) (I2, bool)) *QuerierBuilder[I1, O1, I2, O2] {
	b.decoder = f
	return b
}

func (b *QuerierBuilder[I1, O1, I2, O2]) Encoder(f func(O2) O1) *QuerierBuilder[I1, O1, I2, O2] {
	b.encoder = f
	return b
}

func (b *QuerierBuilder[I1, O1, I2, O2]) Build(f func(session.Query[I2, O2])) Querier[I1, O1, I2, O2] {
	return &transcodingQuerier[I1, O1, I2, O2]{
		ctx:     b.ctx,
		decoder: b.decoder,
		encoder: b.encoder,
		name:    b.name,
		f:       f,
	}
}

var _ ExecutorBuilder[
	session.Query[proto.Message, proto.Message],
	statemachine.QueryID,
	proto.Message,
	proto.Message,
	Querier[proto.Message, proto.Message, proto.Message, proto.Message]] = (*QuerierBuilder[proto.Message, proto.Message, proto.Message, proto.Message])(nil)

type transcodingProposer[I1, O1, I2, O2 proto.Message] struct {
	ctx     Context[I1, O1]
	decoder func(I1) (I2, bool)
	encoder func(O2) O1
	name    string
	f       func(session.Proposal[I2, O2])
}

func (p *transcodingProposer[I1, O1, I2, O2]) Proposals() session.Proposals[I2, O2] {
	return newTranscodingProposals[I1, O1, I2, O2](p.ctx.Proposals(), p.decoder, p.encoder)
}

func (p *transcodingProposer[I1, O1, I2, O2]) Execute(parent session.Proposal[I1, O1]) {
	input, ok := p.decoder(parent.Input())
	if !ok {
		return
	}
	proposal := newTranscodingProposal[I1, O1, I2, O2](parent, input, p.decoder, p.encoder, parent.Log().WithFields(logging.String("Method", p.name)))
	proposal.Log().Debugw("Applying proposal", logging.Stringer("Input", proposal.Input()))
	p.f(proposal)
}

type transcodingQuerier[I1, O1, I2, O2 proto.Message] struct {
	ctx     Context[I1, O1]
	decoder func(I1) (I2, bool)
	encoder func(O2) O1
	name    string
	f       func(session.Query[I2, O2])
}

func (q *transcodingQuerier[I1, O1, I2, O2]) Execute(parent session.Query[I1, O1]) {
	input, ok := q.decoder(parent.Input())
	if !ok {
		return
	}
	query := newTranscodingQuery[I1, O1, I2, O2](parent, input, q.decoder, q.encoder, parent.Log().WithFields(logging.String("Method", q.name)))
	query.Log().Debugw("Applying query", logging.Stringer("Input", query.Input()))
	q.f(query)
}

func newTranscodingSession[I1, O1, I2, O2 any](parent session.Session[I1, O1], decoder func(I1) (I2, bool), encoder func(O2) O1) session.Session[I2, O2] {
	return &transcodingSession[I1, O1, I2, O2]{
		parent:  parent,
		decoder: decoder,
		encoder: encoder,
	}
}

type transcodingSession[I1, O1, I2, O2 any] struct {
	parent  session.Session[I1, O1]
	decoder func(I1) (I2, bool)
	encoder func(O2) O1
}

func (s *transcodingSession[I1, O1, I2, O2]) Log() logging.Logger {
	return s.parent.Log()
}

func (s *transcodingSession[I1, O1, I2, O2]) ID() session.ID {
	return s.parent.ID()
}

func (s *transcodingSession[I1, O1, I2, O2]) State() session.State {
	return s.parent.State()
}

func (s *transcodingSession[I1, O1, I2, O2]) Watch(watcher statemachine.WatchFunc[session.State]) statemachine.CancelFunc {
	return s.parent.Watch(watcher)
}

func (s *transcodingSession[I1, O1, I2, O2]) Proposals() session.Proposals[I2, O2] {
	return newTranscodingProposals[I1, O1, I2, O2](s.parent.Proposals(), s.decoder, s.encoder)
}

func newTranscodingProposals[I1, O1, I2, O2 any](parent session.Proposals[I1, O1], decoder func(I1) (I2, bool), encoder func(O2) O1) session.Proposals[I2, O2] {
	return &transcodingProposals[I1, O1, I2, O2]{
		parent:  parent,
		decoder: decoder,
		encoder: encoder,
	}
}

type transcodingProposals[I1, O1, I2, O2 any] struct {
	parent  session.Proposals[I1, O1]
	decoder func(I1) (I2, bool)
	encoder func(O2) O1
}

func (p *transcodingProposals[I1, O1, I2, O2]) Get(id statemachine.ProposalID) (session.Proposal[I2, O2], bool) {
	parent, ok := p.parent.Get(id)
	if !ok {
		return nil, false
	}
	if input, ok := p.decoder(parent.Input()); ok {
		return newTranscodingProposal[I1, O1, I2, O2](parent, input, p.decoder, p.encoder, parent.Log()), true
	}
	return nil, false
}

func (p *transcodingProposals[I1, O1, I2, O2]) List() []session.Proposal[I2, O2] {
	parents := p.parent.List()
	proposals := make([]session.Proposal[I2, O2], 0, len(parents))
	for _, parent := range parents {
		if input, ok := p.decoder(parent.Input()); ok {
			proposals = append(proposals, newTranscodingProposal[I1, O1, I2, O2](parent, input, p.decoder, p.encoder, parent.Log()))
		}
	}
	return proposals
}

func newTranscodingExecution[T statemachine.ExecutionID, I1, O1, I2, O2 any](
	parent session.Execution[T, I1, O1],
	input I2,
	decoder func(I1) (I2, bool),
	encoder func(O2) O1,
	log logging.Logger) session.Execution[T, I2, O2] {
	return &transcodingExecution[T, I1, O1, I2, O2]{
		parent:  parent,
		input:   input,
		decoder: decoder,
		encoder: encoder,
		log:     log,
	}
}

type transcodingExecution[T statemachine.ExecutionID, I1, O1, I2, O2 any] struct {
	parent  session.Execution[T, I1, O1]
	input   I2
	decoder func(I1) (I2, bool)
	encoder func(O2) O1
	log     logging.Logger
}

func (p *transcodingExecution[T, I1, O1, I2, O2]) ID() T {
	return p.parent.ID()
}

func (p *transcodingExecution[T, I1, O1, I2, O2]) Log() logging.Logger {
	return p.log
}

func (p *transcodingExecution[T, I1, O1, I2, O2]) Session() session.Session[I2, O2] {
	return newTranscodingSession[I1, O1, I2, O2](p.parent.Session(), p.decoder, p.encoder)
}

func (p *transcodingExecution[T, I1, O1, I2, O2]) Watch(watcher statemachine.WatchFunc[statemachine.Phase]) statemachine.CancelFunc {
	return p.parent.Watch(watcher)
}

func (p *transcodingExecution[T, I1, O1, I2, O2]) Input() I2 {
	return p.input
}

func (p *transcodingExecution[T, I1, O1, I2, O2]) Output(output O2) {
	p.parent.Output(p.encoder(output))
}

func (p *transcodingExecution[T, I1, O1, I2, O2]) Error(err error) {
	p.parent.Error(err)
}

func (p *transcodingExecution[T, I1, O1, I2, O2]) Cancel() {
	p.parent.Cancel()
}

func (p *transcodingExecution[T, I1, O1, I2, O2]) Close() {
	p.parent.Close()
}

func newTranscodingProposal[I1, O1, I2, O2 any](
	parent session.Proposal[I1, O1],
	input I2,
	decoder func(I1) (I2, bool),
	encoder func(O2) O1,
	log logging.Logger) session.Proposal[I2, O2] {
	return &transcodingProposal[I2, O2]{
		Execution: newTranscodingExecution[statemachine.ProposalID, I1, O1, I2, O2](parent, input, decoder, encoder, log),
	}
}

type transcodingProposal[I, O any] struct {
	session.Execution[statemachine.ProposalID, I, O]
}

func newTranscodingQuery[I1, O1, I2, O2 any](
	parent session.Query[I1, O1],
	input I2,
	decoder func(I1) (I2, bool),
	encoder func(O2) O1,
	log logging.Logger) session.Query[I2, O2] {
	return &transcodingQuery[I2, O2]{
		Execution: newTranscodingExecution[statemachine.QueryID, I1, O1, I2, O2](parent, input, decoder, encoder, log),
	}
}

type transcodingQuery[I, O any] struct {
	session.Execution[statemachine.QueryID, I, O]
}
