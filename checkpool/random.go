package checkpool

import (
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"
)

const (
	RandomSeedNull = iota
	RandomSeedSecond
	RandomSeedMilli
	RandomSeedMicro
	RandomSeedNano
)

type Random struct {
	sync.RWMutex
	elems  []*randomElement
	totals []int
	max    int
	seed   int
}

type randomElement struct {
	name   string
	weight int
}

func NewRandom() *Random {
	return &Random{
		elems:  make([]*randomElement, 0, 4),
		totals: make([]int, 0, 4),
	}
}

func (e *Random) SetRandomSeed(seed int) {
	e.seed = seed
}

func (e *Random) Add(name string, weight int) {
	e.Lock()
	defer e.Unlock()
	if e.elems == nil {
		e.elems = make([]*randomElement, 0, 4)
	}
	for _, elem := range e.elems {
		if elem.name == name {
			return
		}
	}

	e.elems = append(e.elems, &randomElement{
		weight: weight,
		name:   name,
	})
	e.max += weight
	e.totals = append(e.totals, e.max)
}

func (e *Random) Remove(name string) {
	e.Lock()
	defer e.Unlock()

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

func (e *Random) Update(name string, weight int) error {
	e.Lock()
	defer e.Unlock()
	if e.elems == nil || len(e.elems) == 0 {
		return fmt.Errorf("smooth weight list is empty")
	}

	for _, elem := range e.elems {
		if elem.name == name {
			elem.weight = weight
			return nil
		}
	}
	return fmt.Errorf("element not exist")
}

func (e *Random) Get(name string) (int, error) {
	e.RLock()
	defer e.RUnlock()
	for _, elem := range e.elems {
		if elem.name == name {
			return elem.weight, nil
		}
	}
	return 0, fmt.Errorf("element not exist")
}

func (e *Random) Next() string {
	e.RLock()
	defer e.RUnlock()
	if e.seed != RandomSeedNull {
		if e.seed == RandomSeedSecond {
			rand.Seed(time.Now().Unix())
		} else if e.seed == RandomSeedMilli {
			rand.Seed(time.Now().UnixMilli())
		} else if e.seed == RandomSeedMicro {
			rand.Seed(time.Now().UnixMicro())
		} else if e.seed == RandomSeedNano {
			rand.Seed(time.Now().UnixNano())
		}
	}
	randNum := rand.Intn(e.max) + 1
	i := sort.SearchInts(e.totals, randNum)
	return e.elems[i].name
}

func (e *Random) Reduce(name string) (int, error) {
	e.Lock()
	defer e.Unlock()
	for _, elem := range e.elems {
		if elem.name == name {
			elem.weight--
			return elem.weight, nil
		}
	}
	return 0, fmt.Errorf("element not exist")
}

// All Return all elements
func (e *Random) All() []string {
	e.RLock()
	defer e.RUnlock()
	if e.elems == nil || len(e.elems) == 0 {
		return nil
	}
	all := make([]string, 0, len(e.elems))
	for _, elem := range e.elems {
		all = append(all, elem.name)
	}
	return all
}
