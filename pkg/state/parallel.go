package state

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/torcnet/torcnet/pkg/types"
)

// ParallelExecutionEngine manages parallel execution of state operations
type ParallelExecutionEngine struct {
	workers       int
	workerPool    chan *Worker
	jobQueue      chan *ExecutionJob
	resultQueue   chan *ExecutionResult
	dependencyMap *DependencyMap
	scheduler     *TransactionScheduler
	validator     *ConflictValidator
	metrics       *ExecutionMetrics
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	mu            sync.RWMutex
}

// Worker represents a worker goroutine for parallel execution
type Worker struct {
	id       int
	state    *State
	trie     *MerklePatriciaTrie
	jobChan  chan *ExecutionJob
	stopChan chan struct{}
}

// ExecutionJob represents a job to be executed in parallel
type ExecutionJob struct {
	ID           string
	Transaction  *types.Transaction
	Dependencies []string
	Priority     int
	Timestamp    time.Time
	ResultChan   chan *ExecutionResult
	Context      context.Context
}

// ExecutionResult represents the result of parallel execution
type ExecutionResult struct {
	JobID        string
	Success      bool
	Error        error
	StateChanges map[string][]byte
	GasUsed      uint64
	Logs         []string
	Duration     time.Duration
}

// DependencyMap tracks dependencies between transactions
type DependencyMap struct {
	dependencies map[string][]string
	readSets     map[string]map[string]bool
	writeSets    map[string]map[string]bool
	mu           sync.RWMutex
}

// TransactionScheduler schedules transactions for optimal parallel execution
type TransactionScheduler struct {
	readyQueue    []*ExecutionJob
	waitingQueue  []*ExecutionJob
	executingJobs map[string]*ExecutionJob
	mu            sync.RWMutex
}

// ConflictValidator validates and resolves conflicts between parallel executions
type ConflictValidator struct {
	accessLog     map[string]*AccessRecord
	conflictGraph *ConflictGraph
	mu            sync.RWMutex
}

// AccessRecord tracks access patterns for conflict detection
type AccessRecord struct {
	JobID     string
	ReadKeys  map[string]bool
	WriteKeys map[string]bool
	Timestamp time.Time
}

// ConflictGraph represents conflicts between transactions
type ConflictGraph struct {
	nodes map[string]*ConflictNode
	edges map[string][]string
}

// ConflictNode represents a node in the conflict graph
type ConflictNode struct {
	JobID       string
	Transaction *types.Transaction
	Conflicts   []string
}

// ExecutionMetrics tracks performance metrics
type ExecutionMetrics struct {
	TotalJobs        int64
	CompletedJobs    int64
	FailedJobs       int64
	ConflictCount    int64
	AvgExecutionTime time.Duration
	ThroughputTPS    float64
	ParallelismRate  float64
	mu               sync.RWMutex
}

// NewParallelExecutionEngine creates a new parallel execution engine
func NewParallelExecutionEngine(workers int, state *State, trie *MerklePatriciaTrie) *ParallelExecutionEngine {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	ctx, cancel := context.WithCancel(context.Background())

	engine := &ParallelExecutionEngine{
		workers:       workers,
		workerPool:    make(chan *Worker, workers),
		jobQueue:      make(chan *ExecutionJob, workers*10),
		resultQueue:   make(chan *ExecutionResult, workers*10),
		dependencyMap: NewDependencyMap(),
		scheduler:     NewTransactionScheduler(),
		validator:     NewConflictValidator(),
		metrics:       &ExecutionMetrics{},
		ctx:           ctx,
		cancel:        cancel,
	}

	// Initialize workers
	for i := 0; i < workers; i++ {
		worker := &Worker{
			id:       i,
			state:    state,
			trie:     trie,
			jobChan:  make(chan *ExecutionJob, 10),
			stopChan: make(chan struct{}),
		}
		engine.workerPool <- worker
		engine.wg.Add(1)
		go engine.runWorker(worker)
	}

	// Start scheduler
	engine.wg.Add(1)
	go engine.runScheduler()

	// Start result processor
	engine.wg.Add(1)
	go engine.processResults()

	return engine
}

// ExecuteTransactions executes a batch of transactions in parallel
func (e *ParallelExecutionEngine) ExecuteTransactions(transactions []*types.Transaction) ([]*ExecutionResult, error) {
	if len(transactions) == 0 {
		return nil, nil
	}

	atomic.AddInt64(&e.metrics.TotalJobs, int64(len(transactions)))
	startTime := time.Now()

	// Analyze dependencies
	jobs := e.analyzeAndCreateJobs(transactions)

	// Submit jobs to scheduler
	results := make([]*ExecutionResult, 0, len(jobs))
	resultChans := make([]chan *ExecutionResult, len(jobs))

	for i, job := range jobs {
		resultChans[i] = make(chan *ExecutionResult, 1)
		job.ResultChan = resultChans[i]
		e.scheduler.SubmitJob(job)
	}

	// Collect results
	for i := 0; i < len(jobs); i++ {
		select {
		case result := <-resultChans[i]:
			results = append(results, result)
			if result.Success {
				atomic.AddInt64(&e.metrics.CompletedJobs, 1)
			} else {
				atomic.AddInt64(&e.metrics.FailedJobs, 1)
			}
		case <-time.After(30 * time.Second):
			return nil, fmt.Errorf("execution timeout")
		}
	}

	// Update metrics
	duration := time.Since(startTime)
	e.updateMetrics(len(transactions), duration)

	return results, nil
}

// analyzeAndCreateJobs analyzes transactions and creates execution jobs
func (e *ParallelExecutionEngine) analyzeAndCreateJobs(transactions []*types.Transaction) []*ExecutionJob {
	jobs := make([]*ExecutionJob, len(transactions))

	for i, tx := range transactions {
		job := &ExecutionJob{
			ID:          fmt.Sprintf("job_%d_%s", i, tx.Hash),
			Transaction: tx,
			Priority:    e.calculatePriority(tx),
			Timestamp:   time.Now(),
			Context:     e.ctx,
		}

		// Analyze dependencies
		dependencies := e.dependencyMap.AnalyzeDependencies(tx)
		job.Dependencies = dependencies

		jobs[i] = job
	}

	return jobs
}

// calculatePriority calculates the priority of a transaction
func (e *ParallelExecutionEngine) calculatePriority(tx *types.Transaction) int {
	priority := 0

	// Higher fee = higher priority
	priority += int(tx.Fee / 1000000) // Convert to reasonable scale

	// Lower nonce = higher priority (for same sender)
	priority += int(1000 - tx.Nonce)

	// Contract calls have lower priority than simple transfers
	if len(tx.Data) > 0 {
		priority -= 100
	}

	return priority
}

// runWorker runs a worker goroutine
func (e *ParallelExecutionEngine) runWorker(worker *Worker) {
	defer e.wg.Done()

	for {
		select {
		case job := <-worker.jobChan:
			result := e.executeJob(worker, job)
			job.ResultChan <- result

		case <-worker.stopChan:
			return

		case <-e.ctx.Done():
			return
		}
	}
}

// executeJob executes a single job
func (e *ParallelExecutionEngine) executeJob(worker *Worker, job *ExecutionJob) *ExecutionResult {
	startTime := time.Now()

	result := &ExecutionResult{
		JobID:        job.ID,
		StateChanges: make(map[string][]byte),
		Logs:         make([]string, 0),
	}

	// Create isolated state snapshot for this execution
	snapshot := worker.trie.CreateSnapshot()
	defer func() {
		// Cleanup if execution failed
		if !result.Success {
			worker.trie.RestoreSnapshot(snapshot)
		}
	}()

	// Execute transaction
	err := e.executeTransaction(worker, job.Transaction, result)
	if err != nil {
		result.Error = err
		result.Success = false
	} else {
		result.Success = true
	}

	result.Duration = time.Since(startTime)
	return result
}

// executeTransaction executes a single transaction
func (e *ParallelExecutionEngine) executeTransaction(worker *Worker, tx *types.Transaction, result *ExecutionResult) error {
	// Validate transaction
	if err := e.validateTransaction(tx); err != nil {
		return fmt.Errorf("transaction validation failed: %w", err)
	}

	// Record access patterns for conflict detection
	accessRecord := &AccessRecord{
		JobID:     result.JobID,
		ReadKeys:  make(map[string]bool),
		WriteKeys: make(map[string]bool),
		Timestamp: time.Now(),
	}

	// Execute based on transaction type
	switch {
	case len(tx.Data) == 0:
		// Simple transfer
		return e.executeTransfer(worker, tx, result, accessRecord)
	default:
		// Smart contract execution
		return e.executeContract(worker, tx, result, accessRecord)
	}
}

// executeTransfer executes a simple transfer transaction
func (e *ParallelExecutionEngine) executeTransfer(worker *Worker, tx *types.Transaction, result *ExecutionResult, access *AccessRecord) error {
	fromKey := string(tx.From)
	toKey := string(tx.To)

	// Record access patterns
	access.ReadKeys[fromKey] = true
	access.WriteKeys[fromKey] = true
	access.WriteKeys[toKey] = true

	// Get sender account
	fromAccount, err := worker.state.GetAccount(tx.From)
	if err != nil {
		return fmt.Errorf("failed to get sender account: %w", err)
	}

	// Check balance (amount + fee)
	totalCost := tx.Amount + tx.Fee
	if fromAccount.Balance < totalCost {
		return fmt.Errorf("insufficient balance: %d < %d", fromAccount.Balance, totalCost)
	}

	// Check nonce
	if fromAccount.Nonce != tx.Nonce {
		return fmt.Errorf("invalid nonce: expected %d, got %d", fromAccount.Nonce, tx.Nonce)
	}

	// Get recipient account
	toAccount, err := worker.state.GetAccount(tx.To)
	if err != nil {
		// Create new account if it doesn't exist
		toAccount = &Account{
			Address: tx.To,
			Balance: 0,
			Nonce:   0,
			Storage: make(map[string][]byte),
		}
	}

	// Perform transfer
	fromAccount.Balance -= totalCost
	fromAccount.Nonce++
	toAccount.Balance += tx.Amount

	// Update state
	if err := worker.state.UpdateAccount(fromAccount); err != nil {
		return fmt.Errorf("failed to update sender account: %w", err)
	}

	if err := worker.state.UpdateAccount(toAccount); err != nil {
		return fmt.Errorf("failed to update recipient account: %w", err)
	}

	// Record state changes
	result.StateChanges[fromKey] = fromAccount.Serialize()
	result.StateChanges[toKey] = toAccount.Serialize()
	result.GasUsed = 21000 // Standard transfer gas cost

	// Register access record
	e.validator.RegisterAccess(access)

	return nil
}

// executeContract executes a smart contract transaction
func (e *ParallelExecutionEngine) executeContract(worker *Worker, tx *types.Transaction, result *ExecutionResult, access *AccessRecord) error {
	// This is a simplified smart contract execution
	// In a real implementation, this would involve a VM like EVM

	fromKey := string(tx.From)
	toKey := string(tx.To)

	// Record access patterns
	access.ReadKeys[fromKey] = true
	access.WriteKeys[fromKey] = true

	if len(tx.To) > 0 {
		access.ReadKeys[toKey] = true
		access.WriteKeys[toKey] = true
	}

	// Get sender account
	fromAccount, err := worker.state.GetAccount(tx.From)
	if err != nil {
		return fmt.Errorf("failed to get sender account: %w", err)
	}

	// Check balance for fee (simplified gas model)
	totalCost := tx.Amount + tx.Fee
	if fromAccount.Balance < totalCost {
		return fmt.Errorf("insufficient balance: %d < %d", fromAccount.Balance, totalCost)
	}

	// Deduct cost and increment nonce
	fromAccount.Balance -= totalCost
	fromAccount.Nonce++

	// Simulate contract execution
	gasUsed := e.simulateContractExecution(tx.Data)
	result.GasUsed = gasUsed

	// Update state
	if err := worker.state.UpdateAccount(fromAccount); err != nil {
		return fmt.Errorf("failed to update sender account: %w", err)
	}

	// Record state changes
	result.StateChanges[fromKey] = fromAccount.Serialize()

	// Register access record
	e.validator.RegisterAccess(access)

	return nil
}

// simulateContractExecution simulates smart contract execution
func (e *ParallelExecutionEngine) simulateContractExecution(data []byte) uint64 {
	// This is a simplified simulation
	// In reality, this would involve executing bytecode in a VM
	baseGas := uint64(21000)
	dataGas := uint64(len(data)) * 16 // 16 gas per byte of data

	return baseGas + dataGas
}

// validateTransaction validates a transaction before execution
func (e *ParallelExecutionEngine) validateTransaction(tx *types.Transaction) error {
	// Basic validation
	if tx == nil {
		return fmt.Errorf("transaction is nil")
	}

	if len(tx.From) == 0 {
		return fmt.Errorf("sender address is empty")
	}

	if tx.Amount < 0 {
		return fmt.Errorf("invalid transaction amount")
	}

	if tx.Fee < 0 {
		return fmt.Errorf("invalid transaction fee")
	}

	return nil
}

// runScheduler runs the transaction scheduler
func (e *ParallelExecutionEngine) runScheduler() {
	defer e.wg.Done()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.scheduler.Schedule(e.workerPool, e.validator)

		case <-e.ctx.Done():
			return
		}
	}
}

// processResults processes execution results
func (e *ParallelExecutionEngine) processResults() {
	defer e.wg.Done()

	for {
		select {
		case result := <-e.resultQueue:
			e.handleResult(result)

		case <-e.ctx.Done():
			return
		}
	}
}

// handleResult handles a single execution result
func (e *ParallelExecutionEngine) handleResult(result *ExecutionResult) {
	if result.Success {
		// Apply state changes
		for key, value := range result.StateChanges {
			// This would typically involve committing changes to the main state
			_ = key
			_ = value
		}
	} else {
		// Handle execution failure
		atomic.AddInt64(&e.metrics.ConflictCount, 1)
	}
}

// updateMetrics updates execution metrics
func (e *ParallelExecutionEngine) updateMetrics(jobCount int, duration time.Duration) {
	e.metrics.mu.Lock()
	defer e.metrics.mu.Unlock()

	e.metrics.AvgExecutionTime = duration / time.Duration(jobCount)
	e.metrics.ThroughputTPS = float64(jobCount) / duration.Seconds()

	completed := atomic.LoadInt64(&e.metrics.CompletedJobs)
	total := atomic.LoadInt64(&e.metrics.TotalJobs)
	if total > 0 {
		e.metrics.ParallelismRate = float64(completed) / float64(total)
	}
}

// GetMetrics returns current execution metrics
func (e *ParallelExecutionEngine) GetMetrics() *ExecutionMetrics {
	e.metrics.mu.RLock()
	defer e.metrics.mu.RUnlock()

	return &ExecutionMetrics{
		TotalJobs:        atomic.LoadInt64(&e.metrics.TotalJobs),
		CompletedJobs:    atomic.LoadInt64(&e.metrics.CompletedJobs),
		FailedJobs:       atomic.LoadInt64(&e.metrics.FailedJobs),
		ConflictCount:    atomic.LoadInt64(&e.metrics.ConflictCount),
		AvgExecutionTime: e.metrics.AvgExecutionTime,
		ThroughputTPS:    e.metrics.ThroughputTPS,
		ParallelismRate:  e.metrics.ParallelismRate,
	}
}

// Stop gracefully shuts down the parallel execution engine
func (e *ParallelExecutionEngine) Stop() {
	e.cancel()
	e.wg.Wait()
}

// Dependency Map implementation
func NewDependencyMap() *DependencyMap {
	return &DependencyMap{
		dependencies: make(map[string][]string),
		readSets:     make(map[string]map[string]bool),
		writeSets:    make(map[string]map[string]bool),
	}
}

func (d *DependencyMap) AnalyzeDependencies(tx *types.Transaction) []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	var dependencies []string
	txKey := string(tx.Hash)

	// Analyze read/write sets
	readSet := make(map[string]bool)
	writeSet := make(map[string]bool)

	// Add sender to read/write sets
	readSet[string(tx.From)] = true
	writeSet[string(tx.From)] = true

	// Add recipient to write set
	if len(tx.To) > 0 {
		writeSet[string(tx.To)] = true
	}

	// Check for conflicts with existing transactions
	for existingTx, existingWriteSet := range d.writeSets {
		if existingTx == txKey {
			continue
		}

		// Check for write-after-write or read-after-write conflicts
		for key := range writeSet {
			if existingWriteSet[key] {
				dependencies = append(dependencies, existingTx)
				break
			}
		}

		// Check for write-after-read conflicts
		if existingReadSet, exists := d.readSets[existingTx]; exists {
			for key := range writeSet {
				if existingReadSet[key] {
					dependencies = append(dependencies, existingTx)
					break
				}
			}
		}
	}

	// Store read/write sets
	d.readSets[txKey] = readSet
	d.writeSets[txKey] = writeSet
	d.dependencies[txKey] = dependencies

	return dependencies
}

// Transaction Scheduler implementation
func NewTransactionScheduler() *TransactionScheduler {
	return &TransactionScheduler{
		readyQueue:    make([]*ExecutionJob, 0),
		waitingQueue:  make([]*ExecutionJob, 0),
		executingJobs: make(map[string]*ExecutionJob),
	}
}

func (s *TransactionScheduler) SubmitJob(job *ExecutionJob) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(job.Dependencies) == 0 {
		s.readyQueue = append(s.readyQueue, job)
	} else {
		s.waitingQueue = append(s.waitingQueue, job)
	}
}

func (s *TransactionScheduler) Schedule(workerPool chan *Worker, validator *ConflictValidator) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Process ready queue
	for len(s.readyQueue) > 0 && len(workerPool) > 0 {
		job := s.readyQueue[0]
		s.readyQueue = s.readyQueue[1:]

		select {
		case worker := <-workerPool:
			s.executingJobs[job.ID] = job
			go func(w *Worker, j *ExecutionJob) {
				w.jobChan <- j
				// Return worker to pool after job completion
				workerPool <- w
			}(worker, job)
		default:
			// No workers available, put job back
			s.readyQueue = append([]*ExecutionJob{job}, s.readyQueue...)
			break
		}
	}

	// Check waiting queue for jobs that can be moved to ready queue
	newWaitingQueue := make([]*ExecutionJob, 0)
	for _, job := range s.waitingQueue {
		canExecute := true
		for _, dep := range job.Dependencies {
			if _, executing := s.executingJobs[dep]; executing {
				canExecute = false
				break
			}
		}

		if canExecute {
			s.readyQueue = append(s.readyQueue, job)
		} else {
			newWaitingQueue = append(newWaitingQueue, job)
		}
	}
	s.waitingQueue = newWaitingQueue
}

// Conflict Validator implementation
func NewConflictValidator() *ConflictValidator {
	return &ConflictValidator{
		accessLog: make(map[string]*AccessRecord),
		conflictGraph: &ConflictGraph{
			nodes: make(map[string]*ConflictNode),
			edges: make(map[string][]string),
		},
	}
}

func (v *ConflictValidator) RegisterAccess(access *AccessRecord) {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.accessLog[access.JobID] = access
}

func (v *ConflictValidator) ValidateExecution(jobID string) bool {
	v.mu.RLock()
	defer v.mu.RUnlock()

	access, exists := v.accessLog[jobID]
	if !exists {
		return false
	}

	// Check for conflicts with other concurrent executions
	for otherJobID, otherAccess := range v.accessLog {
		if otherJobID == jobID {
			continue
		}

		if v.hasConflict(access, otherAccess) {
			return false
		}
	}

	return true
}

func (v *ConflictValidator) hasConflict(access1, access2 *AccessRecord) bool {
	// Check for write-write conflicts
	for key := range access1.WriteKeys {
		if access2.WriteKeys[key] {
			return true
		}
	}

	// Check for read-write conflicts
	for key := range access1.ReadKeys {
		if access2.WriteKeys[key] {
			return true
		}
	}

	for key := range access1.WriteKeys {
		if access2.ReadKeys[key] {
			return true
		}
	}

	return false
}