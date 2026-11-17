package registry

// Launch is one launch with its events folded in.
type Launch struct {
	ID string
	// Launch is the first launch event of the id.
	Launch Event
	// End is the last end event of the id, nil when none.
	End Event
}

// Ended reports whether the launch has an end event.
func (l *Launch) Ended() bool { return l.End != nil }

// Fold groups events into launches by id, in launch order. The first launch
// line of an id wins; events of an id with no launch line are ignored.
func Fold(events []Event) []Launch {
	index := map[string]int{}
	var launches []Launch
	for _, e := range events {
		id := e.ID()
		if e.Kind() == KindLaunch {
			if _, seen := index[id]; !seen {
				index[id] = len(launches)
				launches = append(launches, Launch{ID: id, Launch: e})
			}
			continue
		}
		i, ok := index[id]
		if id == "" || !ok {
			continue
		}
		if e.Kind() == KindEnd {
			launches[i].End = e
		}
	}
	return launches
}

// ReadLaunches reads the store and folds its events.
func ReadLaunches(s Store) ([]Launch, error) {
	events, err := s.Events()
	if err != nil {
		return nil, err
	}
	return Fold(events), nil
}

// Find is the launch with the given id.
func Find(launches []Launch, id string) (*Launch, bool) {
	for i := range launches {
		if launches[i].ID == id {
			return &launches[i], true
		}
	}
	return nil, false
}
