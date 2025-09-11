package core

import (
	"github.com/auto-dns/docker-coredns-sync/internal/domain"
	"github.com/auto-dns/docker-coredns-sync/internal/util"
)

// Define struct types
type nestedRecordMap struct {
	*util.DefaultMap[string, *typeMap]
}

type typeMap struct {
	*util.DefaultMap[domain.RecordKind, *recordMap]
}

type recordMap struct {
	*util.DefaultMap[string, *domain.RecordIntent]
}

// Create constructor functions
func newNestedRecordMap() *nestedRecordMap {
	return &nestedRecordMap{
		DefaultMap: util.NewDefaultMap[string](func() *typeMap {
			return newTypeMap()
		}),
	}
}

func newTypeMap() *typeMap {
	return &typeMap{
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
	for _, typeMap := range m.Values() {
		for _, recordMap := range typeMap.Values() {
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

// By name and type
func (m nestedRecordMap) PeekNameTypeRecords(name string, recordType domain.RecordKind) ([]*domain.RecordIntent, bool) {
	if typeMap, exists := m.Peek(name); exists {
		if recordMap, exists := typeMap.Peek(recordType); exists {
			return recordMap.Values(), true
		}
		return []*domain.RecordIntent{}, false
	}
	return []*domain.RecordIntent{}, false
}

// DeleteNameType removes all records of a specific type for a name
func (m nestedRecordMap) DeleteNameType(name string, recordType domain.RecordKind) {
	if domainMap, exists := m.Peek(name); exists {
		domainMap.Delete(recordType)
	}
}

// By name, type, and value
func (m *nestedRecordMap) GetNameTypeRecord(name string, recordType domain.RecordKind, value string) *domain.RecordIntent {
	return m.Get(name).Get(recordType).Get(value)
}

func (m *nestedRecordMap) PeekNameTypeRecord(name string, recordType domain.RecordKind, value string) (*domain.RecordIntent, bool) {
	typeMap, exists := m.Peek(name)
	if !exists {
		return nil, false
	}

	recordMap, exists := typeMap.Peek(recordType)
	if !exists {
		return nil, false
	}

	recordIntent, exists := recordMap.Peek(value)
	return recordIntent, exists
}
