package orm

import (
	"errors"
	"reflect"
	"testing"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
)

type fakeScyllaFindIterator struct {
	columns  []gocql.ColumnInfo
	rows     []map[string]any
	closeErr error
	position int
	closed   bool
}

func (iter *fakeScyllaFindIterator) Columns() []gocql.ColumnInfo {
	return iter.columns
}

func (iter *fakeScyllaFindIterator) MapScan(row map[string]interface{}) bool {
	if iter.position >= len(iter.rows) {
		return false
	}

	for column, value := range iter.rows[iter.position] {
		row[column] = value
	}

	iter.position++

	return true
}

func (iter *fakeScyllaFindIterator) Close() error {
	iter.closed = true
	return iter.closeErr
}

func TestBuildIterScanFindScyllaMapsSingleAndSliceRows(t *testing.T) {
	type record struct {
		ID   int    `gorest:"id"`
		Name string `gorest:"name"`
	}
	columns := []gocql.ColumnInfo{{Name: "id"}, {Name: "name"}}
	lookup := buildStructFieldMapByTag(reflect.TypeFor[record]())

	t.Run("single struct", func(t *testing.T) {
		iter := &fakeScyllaFindIterator{
			columns: columns,
			rows:    []map[string]any{{"id": 7, "name": "Ada"}},
		}
		query := &ORM{}
		destination := reflect.ValueOf(&record{}).Elem()

		if err := query.buildIterScanFindScylla(iter, reflect.TypeFor[record](), lookup, false, &destination, nil); err != nil {
			t.Fatalf("buildIterScanFindScylla() error = %v", err)
		}
		if got := destination.Interface(); got != (record{ID: 7, Name: "Ada"}) {
			t.Fatalf("single row = %#v", got)
		}
		if query.RowsAffected != 1 || !iter.closed {
			t.Fatalf("rows affected = %d, iterator closed = %t", query.RowsAffected, iter.closed)
		}
	})

	t.Run("pointer slice", func(t *testing.T) {
		iter := &fakeScyllaFindIterator{
			columns: columns,
			rows: []map[string]any{
				{"id": 1, "name": "Ada"},
				{"id": 2, "name": "Grace"},
			},
		}
		query := &ORM{}
		var records []*record
		destination := reflect.ValueOf(&records).Elem()
		sliceValue := reflect.MakeSlice(destination.Type(), 0, 0)

		if err := query.buildIterScanFindScylla(iter, reflect.TypeFor[record](), lookup, true, &destination, &sliceValue); err != nil {
			t.Fatalf("buildIterScanFindScylla() error = %v", err)
		}
		got := destination.Interface().([]*record)
		if len(got) != 2 || *got[0] != (record{ID: 1, Name: "Ada"}) || *got[1] != (record{ID: 2, Name: "Grace"}) {
			t.Fatalf("slice rows = %#v", got)
		}
		if query.RowsAffected != 2 || !iter.closed {
			t.Fatalf("rows affected = %d, iterator closed = %t", query.RowsAffected, iter.closed)
		}
	})
}

func TestBuildIterScanFindScyllaReportsMappingAndCloseErrors(t *testing.T) {
	type record struct {
		ID int `gorest:"id"`
	}
	columns := []gocql.ColumnInfo{{Name: "id"}}
	lookup := buildStructFieldMapByTag(reflect.TypeFor[record]())

	t.Run("mapping error", func(t *testing.T) {
		iter := &fakeScyllaFindIterator{
			columns: columns,
			rows:    []map[string]any{{"id": "not-an-integer"}},
		}
		query := &ORM{}
		destination := reflect.ValueOf(&record{}).Elem()

		err := query.buildIterScanFindScylla(iter, reflect.TypeFor[record](), lookup, false, &destination, nil)
		if err == nil || query.Error == nil || !iter.closed {
			t.Fatalf("mapping error = %v, ORM.Error = %v, iterator closed = %t", err, query.Error, iter.closed)
		}
	})

	t.Run("close error", func(t *testing.T) {
		closeErr := errors.New("iterator close failed")
		iter := &fakeScyllaFindIterator{columns: columns, closeErr: closeErr}
		query := &ORM{}
		destination := reflect.ValueOf(&record{}).Elem()

		err := query.buildIterScanFindScylla(iter, reflect.TypeFor[record](), lookup, false, &destination, nil)
		if err == nil || query.Error == nil || !iter.closed {
			t.Fatalf("close error = %v, ORM.Error = %v, iterator closed = %t", err, query.Error, iter.closed)
		}
		if !errors.Is(err, closeErr) {
			t.Fatalf("close error = %v, want wrapped %v", err, closeErr)
		}
	})
}
