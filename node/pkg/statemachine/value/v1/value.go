// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	multiraftv1 "github.com/atomix/multi-raft-storage/api/atomix/multiraft/v1"
	valuev1 "github.com/atomix/multi-raft-storage/api/atomix/multiraft/value/v1"
	"github.com/atomix/multi-raft-storage/node/pkg/snapshot"
	"github.com/atomix/multi-raft-storage/node/pkg/statemachine"
	"github.com/atomix/runtime/sdk/pkg/errors"
	"github.com/gogo/protobuf/proto"
	"sync"
)

const Service = "atomix.runtime.value.v1.Value"

func Register(registry *statemachine.PrimitiveTypeRegistry) {
	statemachine.RegisterPrimitiveType[*valuev1.ValueInput, *valuev1.ValueOutput](registry)(Type)
}

var Type = statemachine.NewPrimitiveType[*valuev1.ValueInput, *valuev1.ValueOutput](Service, valueCodec, newMapStateMachine)

var valueCodec = statemachine.NewCodec[*valuev1.ValueInput, *valuev1.ValueOutput](
	func(bytes []byte) (*valuev1.ValueInput, error) {
		input := &valuev1.ValueInput{}
		if err := proto.Unmarshal(bytes, input); err != nil {
			return nil, err
		}
		return input, nil
	},
	func(output *valuev1.ValueOutput) ([]byte, error) {
		return proto.Marshal(output)
	})

func newMapStateMachine(ctx statemachine.PrimitiveContext[*valuev1.ValueInput, *valuev1.ValueOutput]) statemachine.Primitive[*valuev1.ValueInput, *valuev1.ValueOutput] {
	sm := &MapStateMachine{
		PrimitiveContext: ctx,
		listeners:        make(map[statemachine.ProposalID]bool),
		watchers:         make(map[statemachine.QueryID]statemachine.Query[*valuev1.WatchInput, *valuev1.WatchOutput]),
	}
	sm.init()
	return sm
}

type MapStateMachine struct {
	statemachine.PrimitiveContext[*valuev1.ValueInput, *valuev1.ValueOutput]
	value     *valuev1.ValueState
	listeners map[statemachine.ProposalID]bool
	timer     statemachine.Timer
	watchers  map[statemachine.QueryID]statemachine.Query[*valuev1.WatchInput, *valuev1.WatchOutput]
	mu        sync.RWMutex
	set       statemachine.Updater[*valuev1.ValueInput, *valuev1.ValueOutput, *valuev1.SetInput, *valuev1.SetOutput]
	insert    statemachine.Updater[*valuev1.ValueInput, *valuev1.ValueOutput, *valuev1.InsertInput, *valuev1.InsertOutput]
	update    statemachine.Updater[*valuev1.ValueInput, *valuev1.ValueOutput, *valuev1.UpdateInput, *valuev1.UpdateOutput]
	delete    statemachine.Updater[*valuev1.ValueInput, *valuev1.ValueOutput, *valuev1.DeleteInput, *valuev1.DeleteOutput]
	events    statemachine.Updater[*valuev1.ValueInput, *valuev1.ValueOutput, *valuev1.EventsInput, *valuev1.EventsOutput]
	get       statemachine.Reader[*valuev1.ValueInput, *valuev1.ValueOutput, *valuev1.GetInput, *valuev1.GetOutput]
	watch     statemachine.Reader[*valuev1.ValueInput, *valuev1.ValueOutput, *valuev1.WatchInput, *valuev1.WatchOutput]
}

func (s *MapStateMachine) init() {
	s.set = statemachine.NewUpdater[*valuev1.ValueInput, *valuev1.ValueOutput, *valuev1.SetInput, *valuev1.SetOutput](s).
		Name("Set").
		Decoder(func(input *valuev1.ValueInput) (*valuev1.SetInput, bool) {
			if put, ok := input.Input.(*valuev1.ValueInput_Set); ok {
				return put.Set, true
			}
			return nil, false
		}).
		Encoder(func(output *valuev1.SetOutput) *valuev1.ValueOutput {
			return &valuev1.ValueOutput{
				Output: &valuev1.ValueOutput_Set{
					Set: output,
				},
			}
		}).
		Build(s.doSet)
	s.insert = statemachine.NewUpdater[*valuev1.ValueInput, *valuev1.ValueOutput, *valuev1.InsertInput, *valuev1.InsertOutput](s).
		Name("Insert").
		Decoder(func(input *valuev1.ValueInput) (*valuev1.InsertInput, bool) {
			if put, ok := input.Input.(*valuev1.ValueInput_Insert); ok {
				return put.Insert, true
			}
			return nil, false
		}).
		Encoder(func(output *valuev1.InsertOutput) *valuev1.ValueOutput {
			return &valuev1.ValueOutput{
				Output: &valuev1.ValueOutput_Insert{
					Insert: output,
				},
			}
		}).
		Build(s.doInsert)
	s.update = statemachine.NewUpdater[*valuev1.ValueInput, *valuev1.ValueOutput, *valuev1.UpdateInput, *valuev1.UpdateOutput](s).
		Name("Update").
		Decoder(func(input *valuev1.ValueInput) (*valuev1.UpdateInput, bool) {
			if update, ok := input.Input.(*valuev1.ValueInput_Update); ok {
				return update.Update, true
			}
			return nil, false
		}).
		Encoder(func(output *valuev1.UpdateOutput) *valuev1.ValueOutput {
			return &valuev1.ValueOutput{
				Output: &valuev1.ValueOutput_Update{
					Update: output,
				},
			}
		}).
		Build(s.doUpdate)
	s.delete = statemachine.NewUpdater[*valuev1.ValueInput, *valuev1.ValueOutput, *valuev1.DeleteInput, *valuev1.DeleteOutput](s).
		Name("Delete").
		Decoder(func(input *valuev1.ValueInput) (*valuev1.DeleteInput, bool) {
			if remove, ok := input.Input.(*valuev1.ValueInput_Delete); ok {
				return remove.Delete, true
			}
			return nil, false
		}).
		Encoder(func(output *valuev1.DeleteOutput) *valuev1.ValueOutput {
			return &valuev1.ValueOutput{
				Output: &valuev1.ValueOutput_Delete{
					Delete: output,
				},
			}
		}).
		Build(s.doDelete)
	s.events = statemachine.NewUpdater[*valuev1.ValueInput, *valuev1.ValueOutput, *valuev1.EventsInput, *valuev1.EventsOutput](s).
		Name("Events").
		Decoder(func(input *valuev1.ValueInput) (*valuev1.EventsInput, bool) {
			if events, ok := input.Input.(*valuev1.ValueInput_Events); ok {
				return events.Events, true
			}
			return nil, false
		}).
		Encoder(func(output *valuev1.EventsOutput) *valuev1.ValueOutput {
			return &valuev1.ValueOutput{
				Output: &valuev1.ValueOutput_Events{
					Events: output,
				},
			}
		}).
		Build(s.doEvents)
	s.get = statemachine.NewReader[*valuev1.ValueInput, *valuev1.ValueOutput, *valuev1.GetInput, *valuev1.GetOutput](s).
		Name("Get").
		Decoder(func(input *valuev1.ValueInput) (*valuev1.GetInput, bool) {
			if get, ok := input.Input.(*valuev1.ValueInput_Get); ok {
				return get.Get, true
			}
			return nil, false
		}).
		Encoder(func(output *valuev1.GetOutput) *valuev1.ValueOutput {
			return &valuev1.ValueOutput{
				Output: &valuev1.ValueOutput_Get{
					Get: output,
				},
			}
		}).
		Build(s.doGet)
	s.watch = statemachine.NewReader[*valuev1.ValueInput, *valuev1.ValueOutput, *valuev1.WatchInput, *valuev1.WatchOutput](s).
		Name("Watch").
		Decoder(func(input *valuev1.ValueInput) (*valuev1.WatchInput, bool) {
			if entries, ok := input.Input.(*valuev1.ValueInput_Watch); ok {
				return entries.Watch, true
			}
			return nil, false
		}).
		Encoder(func(output *valuev1.WatchOutput) *valuev1.ValueOutput {
			return &valuev1.ValueOutput{
				Output: &valuev1.ValueOutput_Watch{
					Watch: output,
				},
			}
		}).
		Build(s.doWatch)
}

func (s *MapStateMachine) Snapshot(writer *snapshot.Writer) error {
	s.Log().Infow("Persisting Value to snapshot")
	if err := writer.WriteVarInt(len(s.listeners)); err != nil {
		return err
	}
	for proposalID := range s.listeners {
		if err := writer.WriteVarUint64(uint64(proposalID)); err != nil {
			return err
		}
	}
	if s.value == nil {
		if err := writer.WriteBool(false); err != nil {
			return err
		}
	} else {
		if err := writer.WriteBool(true); err != nil {
			return err
		}
		if err := writer.WriteMessage(s.value); err != nil {
			return err
		}
	}
	return nil
}

func (s *MapStateMachine) Recover(reader *snapshot.Reader) error {
	s.Log().Infow("Recovering Value from snapshot")
	n, err := reader.ReadVarInt()
	if err != nil {
		return err
	}
	for i := 0; i < n; i++ {
		proposalID, err := reader.ReadVarUint64()
		if err != nil {
			return err
		}
		proposal, ok := s.Proposals().Get(statemachine.ProposalID(proposalID))
		if !ok {
			return errors.NewFault("cannot find proposal %d", proposalID)
		}
		s.listeners[proposal.ID()] = true
		proposal.Watch(func(state statemachine.OperationState) {
			if state == statemachine.Complete {
				delete(s.listeners, proposal.ID())
			}
		})
	}

	exists, err := reader.ReadBool()
	if err != nil {
		return err
	}

	if !exists {
		return nil
	}

	state := &valuev1.ValueState{}
	if err := reader.ReadMessage(state); err != nil {
		return err
	}
	s.scheduleTTL(state)
	s.value = state
	return nil
}

func (s *MapStateMachine) Update(proposal statemachine.Proposal[*valuev1.ValueInput, *valuev1.ValueOutput]) {
	switch proposal.Input().Input.(type) {
	case *valuev1.ValueInput_Set:
		s.set.Update(proposal)
	case *valuev1.ValueInput_Insert:
		s.insert.Update(proposal)
	case *valuev1.ValueInput_Update:
		s.update.Update(proposal)
	case *valuev1.ValueInput_Delete:
		s.delete.Update(proposal)
	case *valuev1.ValueInput_Events:
		s.events.Update(proposal)
	default:
		proposal.Error(errors.NewNotSupported("proposal not supported"))
		proposal.Close()
	}
}

func (s *MapStateMachine) doSet(proposal statemachine.Proposal[*valuev1.SetInput, *valuev1.SetOutput]) {
	defer proposal.Close()

	oldValue := s.value
	newValue := &valuev1.ValueState{
		Value: &valuev1.IndexedValue{
			Value: proposal.Input().Value,
			Index: multiraftv1.Index(s.Index()),
		},
	}
	if proposal.Input().TTL != nil {
		expire := s.Scheduler().Time().Add(*proposal.Input().TTL)
		newValue.Expire = &expire
	}

	// Schedule the timeout for the value if necessary.
	s.scheduleTTL(newValue)
	s.value = newValue

	// Publish an event to listener streams.
	if oldValue == nil {
		s.notify(newValue.Value, &valuev1.EventsOutput{
			Event: valuev1.Event{
				Event: &valuev1.Event_Created_{
					Created: &valuev1.Event_Created{
						Value: *newValue.Value,
					},
				},
			},
		})
		proposal.Output(&valuev1.SetOutput{
			Index: newValue.Value.Index,
		})
	} else {
		s.notify(newValue.Value, &valuev1.EventsOutput{
			Event: valuev1.Event{
				Event: &valuev1.Event_Updated_{
					Updated: &valuev1.Event_Updated{
						Value:     *newValue.Value,
						PrevValue: *oldValue.Value,
					},
				},
			},
		})
		proposal.Output(&valuev1.SetOutput{
			Index:     newValue.Value.Index,
			PrevValue: oldValue.Value,
		})
	}
}

func (s *MapStateMachine) doInsert(proposal statemachine.Proposal[*valuev1.InsertInput, *valuev1.InsertOutput]) {
	defer proposal.Close()

	if s.value != nil {
		proposal.Error(errors.NewAlreadyExists("value already set"))
		return
	}

	newValue := &valuev1.ValueState{
		Value: &valuev1.IndexedValue{
			Value: proposal.Input().Value,
			Index: multiraftv1.Index(s.Index()),
		},
	}
	if proposal.Input().TTL != nil {
		expire := s.Scheduler().Time().Add(*proposal.Input().TTL)
		newValue.Expire = &expire
	}

	// Schedule the timeout for the value if necessary.
	s.scheduleTTL(newValue)
	s.value = newValue

	// Publish an event to listener streams.
	s.notify(newValue.Value, &valuev1.EventsOutput{
		Event: valuev1.Event{
			Event: &valuev1.Event_Created_{
				Created: &valuev1.Event_Created{
					Value: *newValue.Value,
				},
			},
		},
	})

	proposal.Output(&valuev1.InsertOutput{
		Index: newValue.Value.Index,
	})
}

func (s *MapStateMachine) doUpdate(proposal statemachine.Proposal[*valuev1.UpdateInput, *valuev1.UpdateOutput]) {
	defer proposal.Close()

	if s.value == nil {
		proposal.Error(errors.NewNotFound("value not set"))
		return
	}

	if proposal.Input().PrevIndex > 0 && s.value.Value.Index != proposal.Input().PrevIndex {
		proposal.Error(errors.NewConflict("value index %d does not match update index %d", s.value.Value.Index, proposal.Input().PrevIndex))
	}

	oldValue := s.value
	newValue := &valuev1.ValueState{
		Value: &valuev1.IndexedValue{
			Value: proposal.Input().Value,
			Index: multiraftv1.Index(s.Index()),
		},
	}
	if proposal.Input().TTL != nil {
		expire := s.Scheduler().Time().Add(*proposal.Input().TTL)
		newValue.Expire = &expire
	}

	// Schedule the timeout for the value if necessary.
	s.scheduleTTL(newValue)
	s.value = newValue

	// Publish an event to listener streams.
	s.notify(newValue.Value, &valuev1.EventsOutput{
		Event: valuev1.Event{
			Event: &valuev1.Event_Updated_{
				Updated: &valuev1.Event_Updated{
					Value:     *newValue.Value,
					PrevValue: *oldValue.Value,
				},
			},
		},
	})

	proposal.Output(&valuev1.UpdateOutput{
		Index:     newValue.Value.Index,
		PrevValue: *oldValue.Value,
	})
}

func (s *MapStateMachine) doDelete(proposal statemachine.Proposal[*valuev1.DeleteInput, *valuev1.DeleteOutput]) {
	defer proposal.Close()

	if s.value == nil {
		proposal.Error(errors.NewNotFound("value not set"))
		return
	}

	if proposal.Input().PrevIndex > 0 && s.value.Value.Index != proposal.Input().PrevIndex {
		proposal.Error(errors.NewConflict("value index %d does not match delete index %d", s.value.Value.Index, proposal.Input().PrevIndex))
	}

	value := s.value
	s.cancelTTL()
	s.value = nil

	// Publish an event to listener streams.
	s.notify(value.Value, &valuev1.EventsOutput{
		Event: valuev1.Event{
			Event: &valuev1.Event_Deleted_{
				Deleted: &valuev1.Event_Deleted{
					Value: *value.Value,
				},
			},
		},
	})

	proposal.Output(&valuev1.DeleteOutput{
		Value: *value.Value,
	})
}

func (s *MapStateMachine) doEvents(proposal statemachine.Proposal[*valuev1.EventsInput, *valuev1.EventsOutput]) {
	// Output an empty event to ack the request
	proposal.Output(&valuev1.EventsOutput{})
	s.listeners[proposal.ID()] = true
	proposal.Watch(func(state statemachine.OperationState) {
		if state == statemachine.Complete {
			delete(s.listeners, proposal.ID())
		}
	})
}

func (s *MapStateMachine) Read(query statemachine.Query[*valuev1.ValueInput, *valuev1.ValueOutput]) {
	switch query.Input().Input.(type) {
	case *valuev1.ValueInput_Get:
		s.get.Read(query)
	case *valuev1.ValueInput_Watch:
		s.watch.Read(query)
	default:
		query.Error(errors.NewNotSupported("query not supported"))
	}
}

func (s *MapStateMachine) doGet(query statemachine.Query[*valuev1.GetInput, *valuev1.GetOutput]) {
	defer query.Close()
	if s.value == nil {
		query.Error(errors.NewNotFound("value not set"))
	} else {
		query.Output(&valuev1.GetOutput{
			Value: s.value.Value,
		})
	}
}

func (s *MapStateMachine) doWatch(query statemachine.Query[*valuev1.WatchInput, *valuev1.WatchOutput]) {
	if s.value != nil {
		query.Output(&valuev1.WatchOutput{
			Value: s.value.Value,
		})
	}

	s.mu.Lock()
	s.watchers[query.ID()] = query
	s.mu.Unlock()
	query.Watch(func(state statemachine.OperationState) {
		if state == statemachine.Complete {
			s.mu.Lock()
			delete(s.watchers, query.ID())
			s.mu.Unlock()
		}
	})
}

func (s *MapStateMachine) notify(value *valuev1.IndexedValue, event *valuev1.EventsOutput) {
	for proposalID := range s.listeners {
		proposal, ok := s.events.Proposals().Get(proposalID)
		if ok {
			proposal.Output(event)
		} else {
			delete(s.listeners, proposalID)
		}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, watcher := range s.watchers {
		watcher.Output(&valuev1.WatchOutput{
			Value: value,
		})
	}
}

func (s *MapStateMachine) scheduleTTL(state *valuev1.ValueState) {
	s.cancelTTL()
	if state.Expire != nil {
		s.timer = s.Scheduler().RunAt(*state.Expire, func() {
			s.value = nil
			s.notify(state.Value, &valuev1.EventsOutput{
				Event: valuev1.Event{
					Event: &valuev1.Event_Deleted_{
						Deleted: &valuev1.Event_Deleted{
							Value:   *state.Value,
							Expired: true,
						},
					},
				},
			})
		})
	}
}

func (s *MapStateMachine) cancelTTL() {
	if s.timer != nil {
		s.timer.Cancel()
		s.timer = nil
	}
}