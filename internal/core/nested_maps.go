package core

import (
	"github.com/auto-dns/docker-coredns-sync/internal/domain"
	"github.com/auto-dns/docker-coredns-sync/internal/util"
)

// Define struct types
type nestedRecordMap struct {
	*util.DefaultMap[string, *kindMap]
}

type kindMap struct {
	*util.DefaultMap[domain.RecordKind, *recordMap]
}

type recordMap struct {
	*util.DefaultMap[string, *domain.RecordIntent]
}

// Create constructor functions
func newNestedRecordMap() *nestedRecordMap {
	return &nestedRecordMap{
		DefaultMap: util.NewDefaultMap[string](func() *kindMap {
			return newkindMap()
		}),
	}
}

func newkindMap() *kindMap {
	return &kindMap{
		DefaultMap: util.NewDefaultMap[domain.RecordKind](func() *recordMap {
			return newRecordMap()
		}),
	}
}

func newRecordMap() *recordMap {
	return &recordMap{
		DefaultMap: util.NewDefaultMap[string](func() *domain.RecordIntent {
			return nil
		}),
	}
}

// By nothing
func (m nestedRecordMap) GetAllValues() []*domain.RecordIntent {
	values := make([]*domain.RecordIntent, 0)
	for _, kindMap := range m.Values() {
		for _, recordMap := range kindMap.Values() {
			for _, recordIntent := range recordMap.Values() {
				values = append(values, recordIntent)
			}
		}
	}
	return values
}

func (m nestedRecordMap) GetAllNames() []string {
	names := make([]string, 0)
	for _, key := range m.Keys() {
		names = append(names, key)
	}
	return names
}

// By name

// By name and kind
func (m nestedRecordMap) PeekNameKindRecords(name string, kind domain.RecordKind) ([]*domain.RecordIntent, bool) {
	if kindMap, exists := m.Peek(name); exists {
		if recordMap, exists := kindMap.Peek(kind); exists {
			return recordMap.Values(), true
		}
		return []*domain.RecordIntent{}, false
	}
	return []*domain.RecordIntent{}, false
}

// DeleteNameKind removes all records of a specific kind for a name
func (m nestedRecordMap) DeleteNameKind(name string, kind domain.RecordKind) {
	if domainMap, exists := m.Peek(name); exists {
		domainMap.Delete(kind)
	}
}

// By name, kind, and value
func (m *nestedRecordMap) GetNameKindRecord(name string, kind domain.RecordKind, value string) *domain.RecordIntent {
	return m.Get(name).Get(kind).Get(value)
}

func (m *nestedRecordMap) PeekNameKindRecord(name string, kind domain.RecordKind, value string) (*domain.RecordIntent, bool) {
	kindMap, exists := m.Peek(name)
	if !exists {
		return nil, false
	}

	recordMap, exists := kindMap.Peek(kind)
	if !exists {
		return nil, false
	}

	recordIntent, exists := recordMap.Peek(value)
	return recordIntent, exists
}
