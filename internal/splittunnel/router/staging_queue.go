package router

import (
	"sync"
	"time"
)

const (
	defaultStagingHoldTimeout = 25 * time.Millisecond
	defaultStagingMaxPackets  = 512
	defaultStagingMaxPerKey   = 8
)

// StagedPacket buffers an early packet (TCP SYN / UDP QUIC Initial) while awaiting PID resolution
// from WINDIVERT_LAYER_SOCKET or GetExtendedTcpTable/UdpTable (Risk R01 mitigation).
type StagedPacket struct {
	Key        SocketKey
	RawPacket  []byte
	IfIdx      uint32
	SubIfIdx   uint32
	Outbound   bool
	EnqueuedAt time.Time
}

// StagingQueue holds packets briefly (15–30ms) to resolve SOCKET vs NETWORK race conditions safely.
type StagingQueue struct {
	mu          sync.Mutex
	queues      map[SocketKey][]StagedPacket
	total       int
	holdTimeout time.Duration
	maxTotal    int
	maxPerKey   int
	nowFn       func() time.Time
}

// NewStagingQueue creates a bounded StagingQueue with a 25ms default hold timeout.
func NewStagingQueue() *StagingQueue {
	return &StagingQueue{
		queues:      make(map[SocketKey][]StagedPacket, 64),
		holdTimeout: defaultStagingHoldTimeout,
		maxTotal:    defaultStagingMaxPackets,
		maxPerKey:   defaultStagingMaxPerKey,
		nowFn:       time.Now,
	}
}

// Enqueue stores a defensive copy of pkt awaiting PID attribution.
// Returns false if global or per-socket queue capacity is exhausted (caller should drop fail-closed).
func (q *StagingQueue) Enqueue(pkt StagedPacket) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.total >= q.maxTotal {
		return false
	}
	bucket := q.queues[pkt.Key]
	if len(bucket) >= q.maxPerKey {
		return false
	}

	cp := pkt
	if len(pkt.RawPacket) > 0 {
		cp.RawPacket = append([]byte(nil), pkt.RawPacket...)
	}
	if cp.EnqueuedAt.IsZero() {
		cp.EnqueuedAt = q.nowFn()
	}

	q.queues[pkt.Key] = append(bucket, cp)
	q.total++
	return true
}

// DrainForSocket pops all staged packets for key in FIFO order once its PID is resolved.
func (q *StagingQueue) DrainForSocket(key SocketKey) []StagedPacket {
	q.mu.Lock()
	defer q.mu.Unlock()

	bucket, ok := q.queues[key]
	if !ok {
		return nil
	}
	delete(q.queues, key)
	q.total -= len(bucket)
	return bucket
}

// PopExpired removes and returns all staged packets older than holdTimeout.
func (q *StagingQueue) PopExpired(now time.Time) []StagedPacket {
	q.mu.Lock()
	defer q.mu.Unlock()

	var expired []StagedPacket
	for key, bucket := range q.queues {
		keep := bucket[:0]
		for _, pkt := range bucket {
			if now.Sub(pkt.EnqueuedAt) >= q.holdTimeout {
				expired = append(expired, pkt)
				q.total--
			} else {
				keep = append(keep, pkt)
			}
		}
		if len(keep) == 0 {
			delete(q.queues, key)
		} else {
			q.queues[key] = keep
		}
	}
	return expired
}

// Clear drops all queued packets.
func (q *StagingQueue) Clear() {
	q.mu.Lock()
	q.queues = make(map[SocketKey][]StagedPacket, 64)
	q.total = 0
	q.mu.Unlock()
}

// Len returns the total number of currently staged packets.
func (q *StagingQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.total
}
