package core

import (
	"github.com/auto-dns/docker-coredns-sync/internal/intent"
	"github.com/auto-dns/docker-coredns-sync/internal/util"
)

// Define struct types
type nestedRecordMap struct {
	*util.DefaultMap[string, *typeMap]
}

type typeMap struct {
	*util.DefaultMap[string, *recordMap]
}

type recordMap struct {
	*util.DefaultMap[string, *intent.RecordIntent]
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
		DefaultMap: util.NewDefaultMap[string](func() *recordMap {
			return newRecordMap()
		}),
	}
}

func newRecordMap() *recordMap {
	return &recordMap{
		DefaultMap: util.NewDefaultMap[string](func() *intent.RecordIntent {
			return nil
		}),
	}
}

// By nothing
func (m nestedRecordMap) GetAllValues() []*intent.RecordIntent {
	values := make([]*intent.RecordIntent, 0)
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
func (m nestedRecordMap) PeekNameTypeRecords(name, recordType string) ([]*intent.RecordIntent, bool) {
	if typeMap, exists := m.Peek(name); exists {
		if recordMap, exists := typeMap.Peek(recordType); exists {
			return recordMap.Values(), true
		}
		return []*intent.RecordIntent{}, false
	}
	return []*intent.RecordIntent{}, false
}

// DeleteDomainType removes all records of a specific type for a name
func (m nestedRecordMap) DeleteNameType(name, recordType string) {
	if domainMap, exists := m.Peek(name); exists {
		domainMap.Delete(recordType)
	}
}

// By name, type, and value
func (m *nestedRecordMap) GetNameTypeRecord(name, recordType, value string) *intent.RecordIntent {
	return m.Get(name).Get(recordType).Get(value)
}

func (m *nestedRecordMap) PeekNameTypeRecord(name, recordType, value string) (*intent.RecordIntent, bool) {
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
