package gomarkov

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"golang.org/x/exp/rand"
)

//Tokens are wrapped around a sequence of words to maintain the
//start and end transition counts
const (
	StartToken = "$"
	EndToken   = "^"
)

var lrnd = rand.New(rand.NewSource(uint64(time.Now().UnixNano())))

//Chain is a markov chain instance
type Chain struct {
	Order        int
	statePool    *spool
	frequencyMat map[int]sparseArray
	lock         *sync.RWMutex
}

type chainJSON struct {
	Order    int                 `json:"int"`
	SpoolMap map[string]int      `json:"spool_map"`
	FreqMat  map[int]sparseArray `json:"freq_mat"`
}

//MarshalJSON ...
func (chain Chain) MarshalJSON() ([]byte, error) {
	obj := chainJSON{
		chain.Order,
		chain.statePool.stringMap,
		chain.frequencyMat,
	}
	return json.Marshal(obj)
}

//UnmarshalJSON ...
func (chain *Chain) UnmarshalJSON(b []byte) error {
	var obj chainJSON
	err := json.Unmarshal(b, &obj)
	if err != nil {
		return err
	}
	chain.Order = obj.Order
	intMap := make(map[int]string)
	for k, v := range obj.SpoolMap {
		intMap[v] = k
	}
	chain.statePool = &spool{
		stringMap: obj.SpoolMap,
		intMap:    intMap,
	}
	chain.frequencyMat = obj.FreqMat
	chain.lock = new(sync.RWMutex)
	return nil
}

//NewChain creates an instance of Chain
func NewChain(order int) *Chain {
	chain := Chain{Order: order}
	chain.statePool = &spool{
		stringMap: make(map[string]int),
		intMap:    make(map[int]string),
	}
	chain.frequencyMat = make(map[int]sparseArray)
	chain.lock = new(sync.RWMutex)
	return &chain
}

//Add adds the transition counts to the chain for a given sequence of words
func (chain *Chain) Add(input []string) {
	startTokens := array(StartToken, chain.Order)
	endTokens := array(EndToken, chain.Order)
	tokens := make([]string, 0)
	tokens = append(tokens, startTokens...)
	tokens = append(tokens, input...)
	tokens = append(tokens, endTokens...)
	pairs := MakePairs(tokens, chain.Order)
	for i := 0; i < len(pairs); i++ {
		pair := pairs[i]
		currentIndex := chain.statePool.add(pair.CurrentState.key())
		nextIndex := chain.statePool.add(pair.NextState)
		chain.lock.Lock()
		if chain.frequencyMat[currentIndex] == nil {
			chain.frequencyMat[currentIndex] = make(sparseArray)
		}
		chain.frequencyMat[currentIndex][nextIndex]++
		chain.lock.Unlock()
	}
}

//TransitionProbability returns the transition probability between two states
func (chain *Chain) TransitionProbability(next string, current NGram) (float64, error) {
	if len(current) != chain.Order {
		return 0, errors.New("n-gram length does not match chain order")
	}
	currentIndex, currentExists := chain.statePool.get(current.key())
	nextIndex, nextExists := chain.statePool.get(next)
	if !currentExists || !nextExists {
		return 0, nil
	}
	arr := chain.frequencyMat[currentIndex]
	sum := float64(arr.sum())
	freq := float64(arr[nextIndex])
	return freq / sum, nil
}

//Generate generates new text based on an initial seed of words
func (chain *Chain) Generate(current NGram) (string, error) {
	if len(current) != chain.Order {
		return "", errors.New("n-gram length does not match chain order")
	}
	if current[len(current)-1] == EndToken {
		// Dont generate anything after the end token
		return "", nil
	}
	currentIndex, currentExists := chain.statePool.get(current.key())
	if !currentExists {
		return "", fmt.Errorf("unknown ngram %v", current)
	}
	arr := chain.frequencyMat[currentIndex]
	sum := arr.sum()
	randN := lrnd.Intn(sum)
	for i, freq := range arr {
		randN -= freq
		if randN <= 0 {
			return chain.statePool.intMap[i], nil
		}
	}
	return "", nil
}

//Generate generates new text based on an initial seed of words
func (chain *Chain) GenerateSeed(current NGram, rnd *rand.Rand) (string, error) {
	if rnd == nil {
		rnd = lrnd
	}
	if len(current) != chain.Order {
		return "", errors.New("n-gram length does not match chain order")
	}
	if current[len(current)-1] == EndToken {
		// Dont generate anything after the end token
		return "", nil
	}
	currentIndex, currentExists := chain.statePool.get(current.key())
	if !currentExists {
		return "", fmt.Errorf("unknown ngram %v", current)
	}
	arr := chain.frequencyMat[currentIndex]
	sum := arr.sum()
	randN := rnd.Intn(sum)
	keys := make([]int, 0, len(arr))
	for k := range arr {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	for _, i := range keys {
		randN -= arr[i]
		if randN <= 0 {
			return chain.statePool.intMap[i], nil
		}
	}
	return "", nil
}
