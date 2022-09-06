// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"container/list"
	"encoding/binary"
	"encoding/json"
	multiraftv1 "github.com/atomix/multi-raft-storage/api/atomix/multiraft/v1"
	"github.com/atomix/multi-raft-storage/node/pkg/statemachine"
	"github.com/atomix/multi-raft-storage/node/pkg/statemachine/snapshot"
	"github.com/atomix/runtime/sdk/pkg/errors"
	"github.com/atomix/runtime/sdk/pkg/logging"
	"github.com/bits-and-blooms/bloom/v3"
	"github.com/google/uuid"
	"time"
)

func newManagedSession(manager *sessionManagerStateMachine) *managedSession {
	return &managedSession{
		manager:   manager,
		proposals: newSessionProposals(),
	}
}

type managedSession struct {
	manager     *sessionManagerStateMachine
	proposals   *sessionProposals
	log         logging.Logger
	id          ID
	state       State
	watchers    map[uuid.UUID]statemachine.WatchFunc[State]
	timeout     time.Duration
	lastUpdated time.Time
	reset       bool
}

func (s *managedSession) Log() logging.Logger {
	return s.log
}

func (s *managedSession) ID() ID {
	return s.id
}

func (s *managedSession) State() State {
	return s.state
}

func (s *managedSession) Watch(f statemachine.WatchFunc[State]) statemachine.CancelFunc {
	id := uuid.New()
	s.watchers[id] = f
	return func() {
		delete(s.watchers, id)
	}
}

func (s *managedSession) Proposals() Proposals {
	return s.proposals
}

func (s *managedSession) Snapshot(writer *snapshot.Writer) error {
	s.Log().Debug("Persisting session to snapshot")
	snapshot := &multiraftv1.SessionSnapshot{
		SessionID:   multiraftv1.SessionID(s.id),
		Timeout:     s.timeout,
		LastUpdated: s.lastUpdated,
	}
	if err := writer.WriteMessage(snapshot); err != nil {
		return err
	}

	proposals := s.proposals.list()
	if err := writer.WriteVarInt(len(proposals)); err != nil {
		return err
	}
	for _, proposal := range proposals {
		if err := proposal.snapshot(writer); err != nil {
			return err
		}
	}
	return nil
}

func (s *managedSession) Recover(reader *snapshot.Reader) error {
	snapshot := &multiraftv1.SessionSnapshot{}
	if err := reader.ReadMessage(snapshot); err != nil {
		return err
	}

	s.id = ID(snapshot.SessionID)
	s.timeout = snapshot.Timeout
	s.lastUpdated = snapshot.LastUpdated

	s.log = s.manager.Log().WithFields(logging.Uint64("Session", uint64(s.id)))
	s.Log().Debug("Recovering session from snapshot")

	n, err := reader.ReadVarInt()
	if err != nil {
		return err
	}
	for i := 0; i < n; i++ {
		proposal := newSessionProposal(s)
		if err := proposal.recover(reader); err != nil {
			return err
		}
	}

	switch snapshot.State {
	case multiraftv1.SessionSnapshot_OPEN:
		s.state = Open
	case multiraftv1.SessionSnapshot_CLOSED:
		s.state = Closed
	}
	s.manager.sessions.add(s)
	return nil
}

func (s *managedSession) checkExpiration(expireTime time.Time) {
	t := s.manager.Time()
	if t.After(expireTime) {
		s.resetTime(t)
	}
	if t.After(s.expireTime()) {
		s.expire()
	}
}

func (s *managedSession) expire() {
	s.Log().Warnf("Session expired after %s", s.manager.Time().Sub(s.lastUpdated))
}

func (s *managedSession) resetTime(t time.Time) {
	if !s.reset {
		s.lastUpdated = t
		s.reset = true
	}
}

func (s *managedSession) expireTime() time.Time {
	return s.lastUpdated.Add(s.timeout)
}

func (s *managedSession) open(proposal statemachine.Proposal[*multiraftv1.OpenSessionInput, *multiraftv1.OpenSessionOutput]) {
	defer proposal.Close()
	s.id = ID(proposal.ID())
	s.state = Open
	s.lastUpdated = s.manager.Time()
	s.timeout = proposal.Input().Timeout
	s.log = s.manager.Log().WithFields(logging.Uint64("Session", uint64(s.id)))
	s.manager.sessions.add(s)
	s.Log().Infow("Opened session", logging.Duration("Timeout", s.timeout))
	proposal.Output(&multiraftv1.OpenSessionOutput{
		SessionID: multiraftv1.SessionID(s.ID()),
	})
}

func (s *managedSession) keepAlive(proposal statemachine.Proposal[*multiraftv1.KeepAliveInput, *multiraftv1.KeepAliveOutput]) {
	defer proposal.Close()

	openInputs := &bloom.BloomFilter{}
	if err := json.Unmarshal(proposal.Input().InputFilter, openInputs); err != nil {
		s.Log().Warn("Failed to decode request filter", err)
		proposal.Error(errors.NewInvalid("invalid request filter", err))
		proposal.Close()
		return
	}

	s.Log().Debug("Processing keep-alive")
	for _, runningProposal := range s.proposals.list() {
		if proposal.Input().LastInputSequenceNum < runningProposal.parent.Input().SequenceNum {
			continue
		}
		sequenceNumBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(sequenceNumBytes, uint64(runningProposal.parent.Input().SequenceNum))
		if !openInputs.Test(sequenceNumBytes) {
			runningProposal.Cancel()
		} else {
			if outputSequenceNum, ok := proposal.Input().LastOutputSequenceNums[runningProposal.parent.Input().SequenceNum]; ok {
				runningProposal.ack(outputSequenceNum)
			}
		}
	}

	proposal.Output(&multiraftv1.KeepAliveOutput{})

	s.lastUpdated = s.manager.Time()
	s.reset = false
}

func (s *managedSession) close(proposal statemachine.Proposal[*multiraftv1.CloseSessionInput, *multiraftv1.CloseSessionOutput]) {
	defer proposal.Close()
	s.manager.sessions.remove(s.id)
	s.state = Closed
	for _, watcher := range s.watchers {
		watcher(Closed)
	}
	proposal.Output(&multiraftv1.CloseSessionOutput{})
}

func newManagedSessions() *managedSessions {
	return &managedSessions{
		sessions: make(map[ID]*managedSession),
	}
}

type managedSessions struct {
	sessions map[ID]*managedSession
}

func (s *managedSessions) Get(id ID) (Session, bool) {
	session, ok := s.sessions[id]
	return session, ok
}

func (s *managedSessions) List() []Session {
	sessions := make([]Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

func (s *managedSessions) add(session *managedSession) bool {
	if _, ok := s.sessions[session.ID()]; !ok {
		s.sessions[session.ID()] = session
		return true
	}
	return false
}

func (s *managedSessions) remove(id ID) bool {
	if _, ok := s.sessions[id]; ok {
		delete(s.sessions, id)
		return true
	}
	return false
}

func (s *managedSessions) get(id ID) (*managedSession, bool) {
	session, ok := s.sessions[id]
	return session, ok
}

func (s *managedSessions) list() []*managedSession {
	sessions := make([]*managedSession, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

func newSessionProposal(session *managedSession) *sessionProposal {
	return &sessionProposal{
		session: session,
		outputs: list.New(),
	}
}

type sessionProposal struct {
	session      *managedSession
	id           statemachine.ProposalID
	input        *multiraftv1.SessionProposalInput
	timer        statemachine.Timer
	phase        statemachine.Phase
	parent       statemachine.Proposal[*multiraftv1.SessionProposalInput, *multiraftv1.SessionProposalOutput]
	watchers     map[uuid.UUID]statemachine.WatchFunc[statemachine.Phase]
	outputs      *list.List
	outputSeqNum multiraftv1.SequenceNum
	log          logging.Logger
}

func (p *sessionProposal) ID() statemachine.ProposalID {
	return p.id
}

func (p *sessionProposal) Log() logging.Logger {
	return p.log
}

func (p *sessionProposal) Session() Session {
	return p.session
}

func (p *sessionProposal) Watch(watcher statemachine.WatchFunc[statemachine.Phase]) statemachine.CancelFunc {
	if p.watchers == nil {
		p.watchers = make(map[uuid.UUID]statemachine.WatchFunc[statemachine.Phase])
	}
	id := uuid.New()
	p.watchers[id] = watcher
	return func() {
		delete(p.watchers, id)
	}
}

func (p *sessionProposal) execute(parent statemachine.Proposal[*multiraftv1.SessionProposalInput, *multiraftv1.SessionProposalOutput]) {
	p.id = parent.ID()
	p.input = parent.Input()
	p.phase = statemachine.Runnnig
	p.log = p.session.Log().WithFields(logging.Uint64("Proposal", uint64(parent.ID())))
	p.parent = parent
	if parent.Input().Deadline != nil {
		p.timer = p.session.manager.Scheduler().Schedule(*parent.Input().Deadline, p.Cancel)
	}

	switch parent.Input().Input.(type) {
	case *multiraftv1.SessionProposalInput_Proposal:
		proposal := newPrimitiveProposal(p)
		p.session.proposals.open(proposal)
		p.session.manager.sm.Propose(proposal)
	case *multiraftv1.SessionProposalInput_CreatePrimitive:
		p.session.proposals.add(p)
		p.session.manager.sm.CreatePrimitive(newCreatePrimitiveProposal(p))
	case *multiraftv1.SessionProposalInput_ClosePrimitive:
		p.session.proposals.add(p)
		p.session.manager.sm.ClosePrimitive(newClosePrimitiveProposal(p))
	}
}

func (p *sessionProposal) replay(parent statemachine.Proposal[*multiraftv1.SessionProposalInput, *multiraftv1.SessionProposalOutput]) {
	p.parent = parent
	if p.outputs.Len() > 0 {
		p.Log().Debug("Replaying proposal outputs")
		elem := p.outputs.Front()
		for elem != nil {
			output := elem.Value.(*multiraftv1.SessionProposalOutput)
			p.parent.Output(output)
			elem = elem.Next()
		}
	}
	if p.phase == statemachine.Complete {
		p.parent.Close()
	}
}

func (p *sessionProposal) snapshot(writer *snapshot.Writer) error {
	p.Log().Info("Persisting proposal to snapshot")
	pendingOutputs := make([]*multiraftv1.SessionProposalOutput, 0, p.outputs.Len())
	elem := p.outputs.Front()
	for elem != nil {
		pendingOutputs = append(pendingOutputs, elem.Value.(*multiraftv1.SessionProposalOutput))
		elem = elem.Next()
	}

	var phase multiraftv1.SessionProposalSnapshot_Phase
	switch p.phase {
	case statemachine.Pending:
		phase = multiraftv1.SessionProposalSnapshot_PENDING
	case statemachine.Runnnig:
		phase = multiraftv1.SessionProposalSnapshot_RUNNING
	case statemachine.Canceled:
		phase = multiraftv1.SessionProposalSnapshot_CANCELED
	case statemachine.Complete:
		phase = multiraftv1.SessionProposalSnapshot_COMPLETE
	}

	snapshot := &multiraftv1.SessionProposalSnapshot{
		Index:                 multiraftv1.Index(p.ID()),
		Phase:                 phase,
		Input:                 p.input,
		PendingOutputs:        pendingOutputs,
		LastOutputSequenceNum: p.outputSeqNum,
	}
	return writer.WriteMessage(snapshot)
}

func (p *sessionProposal) recover(reader *snapshot.Reader) error {
	snapshot := &multiraftv1.SessionProposalSnapshot{}
	if err := reader.ReadMessage(snapshot); err != nil {
		return err
	}
	p.id = statemachine.ProposalID(snapshot.Index)
	p.input = snapshot.Input
	p.log = p.session.Log().WithFields(logging.Uint64("Proposal", uint64(snapshot.Index)))
	p.Log().Info("Recovering command from snapshot")
	p.outputs = list.New()
	for _, output := range snapshot.PendingOutputs {
		r := output
		p.outputs.PushBack(r)
	}
	p.outputSeqNum = snapshot.LastOutputSequenceNum

	switch snapshot.Phase {
	case multiraftv1.SessionProposalSnapshot_PENDING:
		p.phase = statemachine.Pending
	case multiraftv1.SessionProposalSnapshot_RUNNING:
		p.phase = statemachine.Runnnig
		if p.input.Deadline != nil {
			p.timer = p.session.manager.Scheduler().Schedule(*p.input.Deadline, p.Cancel)
		}
	case multiraftv1.SessionProposalSnapshot_COMPLETE:
		p.phase = statemachine.Complete
	case multiraftv1.SessionProposalSnapshot_CANCELED:
		p.phase = statemachine.Canceled
	}

	switch p.input.Input.(type) {
	case *multiraftv1.SessionProposalInput_Proposal:
		proposal := newPrimitiveProposal(p)
		p.session.proposals.open(proposal)
	case *multiraftv1.SessionProposalInput_CreatePrimitive:
		p.session.proposals.add(p)
	case *multiraftv1.SessionProposalInput_ClosePrimitive:
		p.session.proposals.add(p)
	}
	return nil
}

func (p *sessionProposal) ack(outputSequenceNum multiraftv1.SequenceNum) {
	p.Log().Debugw("Acked proposal outputs",
		logging.Uint64("SequenceNum", uint64(outputSequenceNum)))
	elem := p.outputs.Front()
	for elem != nil && elem.Value.(*multiraftv1.SessionProposalOutput).SequenceNum <= outputSequenceNum {
		next := elem.Next()
		p.outputs.Remove(elem)
		elem = next
	}
}

func (p *sessionProposal) nextSequenceNum() multiraftv1.SequenceNum {
	p.outputSeqNum++
	return p.outputSeqNum
}

func (p *sessionProposal) Input() *multiraftv1.SessionProposalInput {
	return p.parent.Input()
}

func (p *sessionProposal) Output(output *multiraftv1.SessionProposalOutput) {
	if p.phase == statemachine.Complete {
		return
	}
	p.Log().Debugw("Cached command output", logging.Uint64("SequenceNum", uint64(output.SequenceNum)))
	p.outputs.PushBack(output)
	p.parent.Output(output)
}

func (p *sessionProposal) Error(err error) {
	if p.phase == statemachine.Complete {
		return
	}
	p.parent.Output(&multiraftv1.SessionProposalOutput{
		SequenceNum: p.nextSequenceNum(),
		Failure:     getFailure(err),
	})
}

func (p *sessionProposal) Close() {
	if p.parent != nil {
		p.parent.Close()
	}
	p.close(statemachine.Complete)
}

func (p *sessionProposal) Cancel() {
	if p.parent != nil {
		p.parent.Cancel()
	}
	p.close(statemachine.Canceled)
}

func (p *sessionProposal) close(phase statemachine.Phase) {
	p.phase = phase
	if p.timer != nil {
		p.timer.Cancel()
	}
	if p.watchers != nil {
		for _, watcher := range p.watchers {
			watcher(phase)
		}
	}
}

func newPrimitiveProposal(parent *sessionProposal) *primitiveProposal {
	return &primitiveProposal{
		sessionProposal: parent,
	}
}

type primitiveProposal struct {
	*sessionProposal
}

func (p *primitiveProposal) Input() *multiraftv1.PrimitiveProposalInput {
	return p.sessionProposal.Input().GetProposal()
}

func (p *primitiveProposal) Output(output *multiraftv1.PrimitiveProposalOutput) {
	p.sessionProposal.Output(&multiraftv1.SessionProposalOutput{
		SequenceNum: p.nextSequenceNum(),
		Output: &multiraftv1.SessionProposalOutput_Proposal{
			Proposal: output,
		},
	})
}

var _ Proposal[*multiraftv1.PrimitiveProposalInput, *multiraftv1.PrimitiveProposalOutput] = (*primitiveProposal)(nil)

func newCreatePrimitiveProposal(parent *sessionProposal) *createPrimitiveProposal {
	return &createPrimitiveProposal{
		sessionProposal: parent,
	}
}

type createPrimitiveProposal struct {
	*sessionProposal
}

func (p *createPrimitiveProposal) Input() *multiraftv1.CreatePrimitiveInput {
	return p.sessionProposal.Input().GetCreatePrimitive()
}

func (p *createPrimitiveProposal) Output(output *multiraftv1.CreatePrimitiveOutput) {
	p.sessionProposal.Output(&multiraftv1.SessionProposalOutput{
		SequenceNum: p.nextSequenceNum(),
		Output: &multiraftv1.SessionProposalOutput_CreatePrimitive{
			CreatePrimitive: output,
		},
	})
}

var _ Proposal[*multiraftv1.CreatePrimitiveInput, *multiraftv1.CreatePrimitiveOutput] = (*createPrimitiveProposal)(nil)

func newClosePrimitiveProposal(parent *sessionProposal) *closePrimitiveProposal {
	return &closePrimitiveProposal{
		sessionProposal: parent,
	}
}

type closePrimitiveProposal struct {
	*sessionProposal
}

func (p *closePrimitiveProposal) Input() *multiraftv1.ClosePrimitiveInput {
	return p.sessionProposal.Input().GetClosePrimitive()
}

func (p *closePrimitiveProposal) Output(output *multiraftv1.ClosePrimitiveOutput) {
	p.sessionProposal.Output(&multiraftv1.SessionProposalOutput{
		SequenceNum: p.nextSequenceNum(),
		Output: &multiraftv1.SessionProposalOutput_ClosePrimitive{
			ClosePrimitive: output,
		},
	})
}

var _ Proposal[*multiraftv1.ClosePrimitiveInput, *multiraftv1.ClosePrimitiveOutput] = (*closePrimitiveProposal)(nil)

func newSessionProposals() *sessionProposals {
	return &sessionProposals{
		primitiveProposals: make(map[statemachine.ProposalID]*primitiveProposal),
		sessionProposals:   make(map[multiraftv1.SequenceNum]*sessionProposal),
	}
}

type sessionProposals struct {
	primitiveProposals map[statemachine.ProposalID]*primitiveProposal
	sessionProposals   map[multiraftv1.SequenceNum]*sessionProposal
}

func (p *sessionProposals) Get(id statemachine.ProposalID) (Proposal[*multiraftv1.PrimitiveProposalInput, *multiraftv1.PrimitiveProposalOutput], bool) {
	proposal, ok := p.primitiveProposals[id]
	return proposal, ok
}

func (p *sessionProposals) List() []Proposal[*multiraftv1.PrimitiveProposalInput, *multiraftv1.PrimitiveProposalOutput] {
	proposals := make([]Proposal[*multiraftv1.PrimitiveProposalInput, *multiraftv1.PrimitiveProposalOutput], 0, len(p.primitiveProposals))
	for _, proposal := range p.primitiveProposals {
		proposals = append(proposals, proposal)
	}
	return proposals
}

func (p *sessionProposals) open(proposal *primitiveProposal) {
	p.primitiveProposals[proposal.ID()] = proposal
	p.add(proposal.sessionProposal)
}

func (p *sessionProposals) close(id statemachine.ProposalID) {
	delete(p.primitiveProposals, id)
}

func (p *sessionProposals) add(proposal *sessionProposal) {
	p.sessionProposals[proposal.Input().SequenceNum] = proposal
}

func (p *sessionProposals) remove(seqNum multiraftv1.SequenceNum) {
	delete(p.sessionProposals, seqNum)
}

func (p *sessionProposals) get(seqNum multiraftv1.SequenceNum) (*sessionProposal, bool) {
	proposal, ok := p.sessionProposals[seqNum]
	return proposal, ok
}

func (p *sessionProposals) list() []*sessionProposal {
	proposals := make([]*sessionProposal, 0, len(p.sessionProposals))
	for _, proposal := range p.sessionProposals {
		proposals = append(proposals, proposal)
	}
	return proposals
}

func newSessionQuery(session *managedSession) *sessionQuery {
	return &sessionQuery{
		session: session,
	}
}

type sessionQuery struct {
	session *managedSession
	parent  statemachine.Query[*multiraftv1.SessionQueryInput, *multiraftv1.SessionQueryOutput]
	phase   statemachine.Phase
	log     logging.Logger
}

func (p *sessionQuery) ID() statemachine.QueryID {
	return p.parent.ID()
}

func (p *sessionQuery) Log() logging.Logger {
	return p.log
}

func (p *sessionQuery) Session() Session {
	return p.session
}

func (p *sessionQuery) Watch(watcher statemachine.WatchFunc[statemachine.Phase]) statemachine.CancelFunc {
	return p.parent.Watch(watcher)
}

func (p *sessionQuery) execute(proposal statemachine.Query[*multiraftv1.SessionQueryInput, *multiraftv1.SessionQueryOutput]) {
	p.parent = proposal
	p.log = p.session.Log().WithFields(logging.Uint64("Query", uint64(proposal.ID())))
}

func (p *sessionQuery) Input() *multiraftv1.PrimitiveQueryInput {
	return p.parent.Input().GetQuery()
}

func (p *sessionQuery) Output(output *multiraftv1.PrimitiveQueryOutput) {
	p.parent.Output(&multiraftv1.SessionQueryOutput{
		Output: &multiraftv1.SessionQueryOutput_Query{
			Query: output,
		},
	})
}

func (p *sessionQuery) Error(err error) {
	p.parent.Error(err)
}

func (p *sessionQuery) Cancel() {
	p.parent.Cancel()
}

func (p *sessionQuery) Close() {
	p.parent.Close()
}

var _ Query[*multiraftv1.PrimitiveQueryInput, *multiraftv1.PrimitiveQueryOutput] = (*sessionQuery)(nil)

// getFailure gets the proto status for the given error
func getFailure(err error) *multiraftv1.Failure {
	if err == nil {
		return nil
	}
	return &multiraftv1.Failure{
		Status:  getStatus(err),
		Message: getMessage(err),
	}
}

func getStatus(err error) multiraftv1.Failure_Status {
	typed, ok := err.(*errors.TypedError)
	if !ok {
		return multiraftv1.Failure_ERROR
	}

	switch typed.Type {
	case errors.Unknown:
		return multiraftv1.Failure_UNKNOWN
	case errors.Canceled:
		return multiraftv1.Failure_CANCELED
	case errors.NotFound:
		return multiraftv1.Failure_NOT_FOUND
	case errors.AlreadyExists:
		return multiraftv1.Failure_ALREADY_EXISTS
	case errors.Unauthorized:
		return multiraftv1.Failure_UNAUTHORIZED
	case errors.Forbidden:
		return multiraftv1.Failure_FORBIDDEN
	case errors.Conflict:
		return multiraftv1.Failure_CONFLICT
	case errors.Invalid:
		return multiraftv1.Failure_INVALID
	case errors.Unavailable:
		return multiraftv1.Failure_UNAVAILABLE
	case errors.NotSupported:
		return multiraftv1.Failure_NOT_SUPPORTED
	case errors.Timeout:
		return multiraftv1.Failure_TIMEOUT
	case errors.Fault:
		return multiraftv1.Failure_FAULT
	case errors.Internal:
		return multiraftv1.Failure_INTERNAL
	default:
		return multiraftv1.Failure_ERROR
	}
}

// getMessage gets the message for the given error
func getMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}