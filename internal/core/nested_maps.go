package core

import (
	"github.com/StevenC4/docker-coredns-sync/internal/intent"
	"github.com/StevenC4/docker-coredns-sync/internal/util"
)

// Define struct types
type NestedRecordMap struct {
	*util.DefaultMap[string, *TypeMap]
}

type TypeMap struct {
	*util.DefaultMap[string, *RecordMap]
}

type RecordMap struct {
	*util.DefaultMap[string, *intent.RecordIntent]
}

// Create constructor functions
func NewNestedRecordMap() *NestedRecordMap {
	return &NestedRecordMap{
		DefaultMap: util.NewDefaultMap[string](func() *TypeMap {
			return NewTypeMap()
		}),
	}
}

func NewTypeMap() *TypeMap {
	return &TypeMap{
		DefaultMap: util.NewDefaultMap[string](func() *RecordMap {
			return NewRecordMap()
		}),
	}
}

func NewRecordMap() *RecordMap {
	return &RecordMap{
		DefaultMap: util.NewDefaultMap[string](func() *intent.RecordIntent {
			return nil
		}),
	}
}

// By nothing
func (m NestedRecordMap) GetAllValues() []*intent.RecordIntent {
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

func (m NestedRecordMap) GetAllNames() []string {
	names := make([]string, 0)
	for _, key := range m.Keys() {
		names = append(names, key)
	}
	return names
}

// By name

// By name and type
func (m NestedRecordMap) PeekNameTypeRecords(name, recordType string) ([]*intent.RecordIntent, bool) {
	if typeMap, exists := m.Peek(name); exists {
		if recordMap, exists := typeMap.Peek(recordType); exists {
			return recordMap.Values(), true
		}
		return []*intent.RecordIntent{}, false
	}
	return []*intent.RecordIntent{}, false
}

// DeleteDomainType removes all records of a specific type for a name
func (m NestedRecordMap) DeleteNameType(name, recordType string) {
	if domainMap, exists := m.Peek(name); exists {
		domainMap.Delete(recordType)
	}
}

// By name, type, and value
func (m *NestedRecordMap) GetNameTypeRecord(name, recordType, value string) *intent.RecordIntent {
	return m.Get(name).Get(recordType).Get(value)
}

func (m *NestedRecordMap) PeekNameTypeRecord(name, recordType, value string) (*intent.RecordIntent, bool) {
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
