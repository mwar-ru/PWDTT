package appmgr

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultResolverTTL      = 15 * time.Second
	defaultMaxCacheEntries  = 4096
	defaultMaxParentDepth   = 8
)

// ErrProcessNotFound is returned when a PID no longer exists or cannot be queried.
var ErrProcessNotFound = errors.New("process not found or access denied")

// RunningApp represents a deduplicated running application for the UI app picker.
type RunningApp struct {
	Name           string   `json:"name"`
	ExePath        string   `json:"exe_path"`
	NormalizedPath string   `json:"normalized_path"`
	ProcessCount   int      `json:"process_count"`
	PIDs           []uint32 `json:"pids"`
}

type rawProcessInfo struct {
	PID          uint32
	PPID         uint32
	CreationTime uint64
	ExePath      string
}

type processBackend interface {
	LookupProcess(pid uint32) (rawProcessInfo, error)
	ListProcesses() ([]rawProcessInfo, error)
}

type cachedProcessEntry struct {
	info         ProcessInfo
	creationTime uint64
	cachedAt     time.Time
}

// ProcessResolver resolves PID -> canonical EXE path + ancestor chain with PID-reuse protection.
type ProcessResolver struct {
	backend         processBackend
	ttl             time.Duration
	maxCacheEntries int
	maxParentDepth  int
	nowFn           func() time.Time

	mu    sync.RWMutex
	cache map[uint32]cachedProcessEntry
}

// NewProcessResolverWithBackend builds a ProcessResolver using the given backend (primarily for tests).
func NewProcessResolverWithBackend(backend processBackend) *ProcessResolver {
	return &ProcessResolver{
		backend:         backend,
		ttl:             defaultResolverTTL,
		maxCacheEntries: defaultMaxCacheEntries,
		maxParentDepth:  defaultMaxParentDepth,
		nowFn:           time.Now,
		cache:           make(map[uint32]cachedProcessEntry, 256),
	}
}

// Resolve returns ProcessInfo (canonical ExePath + ParentChain) for the given PID.
func (r *ProcessResolver) Resolve(pid uint32) (ProcessInfo, error) {
	if pid == 0 {
		return ProcessInfo{}, ErrProcessNotFound
	}

	now := r.nowFn()
	r.mu.RLock()
	entry, ok := r.cache[pid]
	r.mu.RUnlock()
	if ok && now.Sub(entry.cachedAt) < r.ttl {
		return cloneProcessInfo(entry.info), nil
	}

	raw, err := r.backend.LookupProcess(pid)
	if err != nil {
		return ProcessInfo{}, err
	}

	normExe := NormalizeWindowsPath(raw.ExePath)
	if normExe == "" {
		return ProcessInfo{}, ErrProcessNotFound
	}

	parentChain := r.resolveParentChain(raw)
	info := ProcessInfo{
		PID:         pid,
		ExePath:     normExe,
		ParentChain: parentChain,
	}

	r.mu.Lock()
	if len(r.cache) >= r.maxCacheEntries {
		r.evictExpiredLocked(now)
		if len(r.cache) >= r.maxCacheEntries {
			// Bounded fallback reset so cache memory can never grow unbounded
			r.cache = make(map[uint32]cachedProcessEntry, 256)
		}
	}
	r.cache[pid] = cachedProcessEntry{
		info:         cloneProcessInfo(info),
		creationTime: raw.CreationTime,
		cachedAt:     now,
	}
	r.mu.Unlock()

	return info, nil
}

func (r *ProcessResolver) resolveParentChain(child rawProcessInfo) []string {
	var chain []string
	visited := map[uint32]struct{}{
		child.PID: {},
	}

	curPPID := child.PPID
	curCreation := child.CreationTime

	for depth := 0; depth < r.maxParentDepth; depth++ {
		if curPPID == 0 || curPPID == 4 {
			break
		}
		if _, seen := visited[curPPID]; seen {
			break
		}
		visited[curPPID] = struct{}{}

		parentRaw, err := r.backend.LookupProcess(curPPID)
		if err != nil {
			break
		}

		// PID-reuse guard: a legitimate parent process cannot be created AFTER its child.
		if curCreation > 0 && parentRaw.CreationTime > 0 && parentRaw.CreationTime > curCreation {
			break
		}

		parentNorm := NormalizeWindowsPath(parentRaw.ExePath)
		if parentNorm != "" {
			chain = append(chain, parentNorm)
		}

		curPPID = parentRaw.PPID
		if parentRaw.CreationTime > 0 {
			curCreation = parentRaw.CreationTime
		}
	}

	return chain
}

func (r *ProcessResolver) evictExpiredLocked(now time.Time) {
	for pid, entry := range r.cache {
		if now.Sub(entry.cachedAt) >= r.ttl {
			delete(r.cache, pid)
		}
	}
}

// Invalidate drops a PID from the resolver cache (e.g. when a socket closes or process exits).
func (r *ProcessResolver) Invalidate(pid uint32) {
	r.mu.Lock()
	delete(r.cache, pid)
	r.mu.Unlock()
}

// ClearCache empties the entire resolver cache.
func (r *ProcessResolver) ClearCache() {
	r.mu.Lock()
	r.cache = make(map[uint32]cachedProcessEntry, 256)
	r.mu.Unlock()
}

// ListRunningApps scans active user-accessible processes and groups multi-process apps
// (such as Chrome, Edge, Firefox) into a single deduplicated entry per executable path.
func (r *ProcessResolver) ListRunningApps() ([]RunningApp, error) {
	rawList, err := r.backend.ListProcesses()
	if err != nil {
		return nil, err
	}

	grouped := make(map[string]*RunningApp, len(rawList))
	for _, proc := range rawList {
		norm := NormalizeWindowsPath(proc.ExePath)
		if norm == "" {
			continue
		}
		existing, ok := grouped[norm]
		if !ok {
			displayName := prettyAppName(proc.ExePath)
			grouped[norm] = &RunningApp{
				Name:           displayName,
				ExePath:        strings.TrimSpace(proc.ExePath),
				NormalizedPath: norm,
				ProcessCount:   1,
				PIDs:           []uint32{proc.PID},
			}
			continue
		}
		existing.ProcessCount++
		existing.PIDs = append(existing.PIDs, proc.PID)
	}

	result := make([]RunningApp, 0, len(grouped))
	for _, app := range grouped {
		sort.Slice(app.PIDs, func(i, j int) bool { return app.PIDs[i] < app.PIDs[j] })
		result = append(result, *app)
	}

	sort.Slice(result, func(i, j int) bool {
		li := strings.ToLower(result[i].Name)
		lj := strings.ToLower(result[j].Name)
		if li != lj {
			return li < lj
		}
		return result[i].NormalizedPath < result[j].NormalizedPath
	})

	return result, nil
}

func prettyAppName(exePath string) string {
	base := BaseExeName(exePath)
	if base == "" {
		base = filepath.Base(strings.TrimSpace(exePath))
	}
	trimmed := strings.TrimSuffix(base, ".exe")
	if trimmed == "" {
		return base
	}
	return strings.ToUpper(trimmed[:1]) + trimmed[1:]
}

func cloneProcessInfo(in ProcessInfo) ProcessInfo {
	cp := in
	if len(in.ParentChain) > 0 {
		cp.ParentChain = append([]string(nil), in.ParentChain...)
	}
	return cp
}
