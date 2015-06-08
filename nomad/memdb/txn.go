package memdb

import (
	"fmt"

	"github.com/hashicorp/go-immutable-radix"
)

// tableIndex is a tuple of (Table, Index) used for lookups
type tableIndex struct {
	Table string
	Index string
}

// Txn is a transaction against a MemDB. This can be a read or write transaction.
type Txn struct {
	db      *MemDB
	write   bool
	rootTxn *iradix.Txn

	modified map[tableIndex]*iradix.Txn
}

// readableIndex returns a transaction usable for reading the given
// index in a table. If a write transaction is in progress, we may need
// to use an existing modified txn.
func (txn *Txn) readableIndex(table, index string) *iradix.Txn {
	// Look for existing transaction
	if txn.write && txn.modified != nil {
		key := tableIndex{table, index}
		exist, ok := txn.modified[key]
		if ok {
			return exist
		}
	}

	// Create a read transaction
	path := indexPath(table, index)
	raw, _ := txn.rootTxn.Get(path)
	indexRoot := toTree(raw)
	indexTxn := indexRoot.Txn()
	return indexTxn
}

// writableIndex returns a transaction usable for modifying the
// given index in a table.
func (txn *Txn) writableIndex(table, index string) *iradix.Txn {
	if txn.modified == nil {
		txn.modified = make(map[tableIndex]*iradix.Txn)
	}

	// Look for existing transaction
	key := tableIndex{table, index}
	exist, ok := txn.modified[key]
	if ok {
		return exist
	}

	// Start a new transaction
	path := indexPath(table, index)
	raw, _ := txn.rootTxn.Get(path)
	indexRoot := toTree(raw)
	indexTxn := indexRoot.Txn()

	// Keep this open for the duration of the txn
	txn.modified[key] = indexTxn
	return indexTxn
}

// Abort is used to cancel this transaction. This is a noop for read transactions.
func (txn *Txn) Abort() {
	// Noop for a read transaction
	if !txn.write {
		return
	}

	// Check if already aborted or committed
	if txn.rootTxn == nil {
		return
	}

	// Clear the txn
	txn.rootTxn = nil
	txn.modified = nil

	// Release the writer lock since this is invalid
	txn.db.writer.Unlock()
}

// Commit is used to finalize this transaction. This is a noop for read transactions.
func (txn *Txn) Commit() {
	// Noop for a read transaction
	if !txn.write {
		return
	}

	// Check if already aborted or committed
	if txn.rootTxn == nil {
		return
	}

	// Commit each sub-transaction scoped to (table, index)
	for key, subTxn := range txn.modified {
		path := indexPath(key.Table, key.Index)
		txn.rootTxn.Insert(path, subTxn.Commit())
	}

	// Update the root of the DB
	txn.db.root = txn.rootTxn.Commit()

	// Clear the txn
	txn.rootTxn = nil
	txn.modified = nil

	// Release the writer lock since this is invalid
	txn.db.writer.Unlock()
}

// Insert is used to add or update an object into the given table
func (txn *Txn) Insert(table string, obj interface{}) error {
	if !txn.write {
		return fmt.Errorf("cannot insert in read-only transaction")
	}

	// Get the table schema
	tableSchema, ok := txn.db.schema.Tables[table]
	if !ok {
		return fmt.Errorf("invalid table '%s'", table)
	}

	// Lookup the object by ID first
	// TODO: Handle delete if existing (update)

	for name, indexSchema := range tableSchema.Indexes {
		ok, val, err := indexSchema.Indexer.FromObject(obj)
		if err != nil {
			return fmt.Errorf("failed to build index '%s': %v", name, err)
		}
		if !ok {
			if indexSchema.AllowMissing {
				continue
			} else {
				return fmt.Errorf("missing value for index '%s'", name)
			}
		}

		// TODO: Handle non-unique index
		indexTxn := txn.writableIndex(table, name)
		indexTxn.Insert(val, obj)
	}
	return nil
}

func (txn *Txn) Delete(table, index string, args ...interface{}) error {
	if !txn.write {
		return fmt.Errorf("cannot delete in read-only transaction")
	}
	return nil
}

func (txn *Txn) First(table, index string, args ...interface{}) (interface{}, error) {
	// Get the table schema
	tableSchema, ok := txn.db.schema.Tables[table]
	if !ok {
		return nil, fmt.Errorf("invalid table '%s'", table)
	}

	// Get the index schema
	indexSchema, ok := tableSchema.Indexes[index]
	if !ok {
		return nil, fmt.Errorf("invalid index '%s'", index)
	}

	// Get the exact match index
	val, err := indexSchema.Indexer.FromArgs(args...)
	if err != nil {
		return nil, fmt.Errorf("index error: %v", err)
	}

	// Get the index itself
	indexTxn := txn.readableIndex(table, index)

	// Do an exact lookup
	obj, ok := indexTxn.Get(val)
	if !ok {
		return nil, nil
	}

	// TODO: handle non-unique index
	return obj, nil
}

type ResultIterator interface {
	Next() interface{}
}

func (txn *Txn) Get(table, index string, args ...interface{}) (ResultIterator, error) {
	return nil, nil
}

// toTree is used to do a fast assertion of type in cases
// where it is known to avoid the overhead of reflection
func toTree(raw interface{}) *iradix.Tree {
	return raw.(*iradix.Tree)
	// TODO: Fix this
	//return (*iradix.Tree)(raw.(unsafe.Pointer))
}
