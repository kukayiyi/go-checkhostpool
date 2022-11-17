package checkpool

import (
	"fmt"
	"sync"
)

// RoundRobin smooth weighted round-robin implement
type RoundRobin struct {
	mtx   sync.RWMutex
	elems []*roundRobinElem
}

// roundRobinElem element of smooth weight round-robin implement
type roundRobinElem struct {
	weight          int
	currentWeight   int
	effectiveWeight int
	name            string
}

func NewRoundRobin() *RoundRobin {
	return &RoundRobin{
		elems: make([]*roundRobinElem, 0, 4),
	}
}

// Add a naming element with weight
func (e *RoundRobin) Add(name string, weight int) {
	e.mtx.Lock()
	defer e.mtx.Unlock()
	if e.elems == nil {
		e.elems = make([]*roundRobinElem, 0, 4)
	}

	for _, elem := range e.elems {
		if elem.name == name {
			return
		}
	}

	e.elems = append(e.elems, &roundRobinElem{
		weight:          weight,
		currentWeight:   0,
		effectiveWeight: weight,
		name:            name,
	})
}

// Remove a single element by its name and return
// error if empty elements or element not exist
func (e *RoundRobin) Remove(name string) {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	if e.elems == nil || len(e.elems) == 0 {
		return
	}

	for idx, elem := range e.elems {
		if elem.name == name {
			e.elems = append(e.elems[0:idx], e.elems[idx+1:]...)
			return
		}
	}
}

// Update weight of the element dynamically
func (e *RoundRobin) Update(name string, weight int) error {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	if e.elems == nil || len(e.elems) == 0 {
		return fmt.Errorf("weight list is empty")
	}

	for _, elem := range e.elems {
		if elem.name == name {
			elem.weight = weight
			elem.effectiveWeight = weight
			elem.currentWeight = 0
			return nil
		}
	}
	return fmt.Errorf("element not exist")
}

// Get the weight of element
func (e *RoundRobin) Get(name string) (int, error) {
	e.mtx.RLock()
	defer e.mtx.RUnlock()
	for _, elem := range e.elems {
		if elem.name == name {
			return elem.effectiveWeight, nil
		}
	}
	return 0, fmt.Errorf("element not exist")
}

// Next pick up next element under smooth round-robin weight balancing
func (e *RoundRobin) Next() string {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	if e.elems == nil || len(e.elems) == 0 {
		return ""
	}

	var total int
	next := &roundRobinElem{}
	for _, elem := range e.elems {
		total += elem.effectiveWeight
		elem.currentWeight += elem.effectiveWeight

		if elem.effectiveWeight < elem.weight { // automatic recovery
			elem.effectiveWeight++
		}

		if next == nil || next.currentWeight < elem.currentWeight {
			next = elem
		}
	}

	next.currentWeight -= total
	return next.name
}

// Reduce effective weight for element by name, usually used to report communication errors
// return weight after reduce, and error if element not exist
func (e *RoundRobin) Reduce(name string) (int, error) {
	e.mtx.Lock()
	defer e.mtx.Unlock()
	for _, elem := range e.elems {
		if elem.name == name {
			elem.effectiveWeight /= 2
			return elem.effectiveWeight, nil
		}
	}
	return 0, fmt.Errorf("element not exist")
}

// All Return all elements
func (e *RoundRobin) All() []string {
	e.mtx.RLock()
	defer e.mtx.RUnlock()
	if e.elems == nil || len(e.elems) == 0 {
		return nil
	}
	all := make([]string, 0, len(e.elems))
	for _, elem := range e.elems {
		all = append(all, elem.name)
	}
	return all
}
