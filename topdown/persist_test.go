package topdown

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/cockroachdb/pebble"
	"github.com/meta-quick/opax/ast"
	"github.com/meta-quick/opax/storage"
	"github.com/meta-quick/opax/storage/inmem"
	"testing"
	"time"
)

func TestPersist(t *testing.T) {
	// Register the storage
	store := NewPebbleStorage("/tmp/testx.db", pebble.Options{})
	store.SetInteger("key", 100)
	store.SetString("key2", "xxx")
	value2, _ := store.GetString("key2")
	value1, _ := store.GetInteger("key")

	println("value", value1, value2)
	// Close the storage
}

func TestCounters(t *testing.T) {
	c := NewGauge(10)
	for i := 0; i < 20; i++ {
		//time.Sleep(time.Second * 1)
		c.Add(int64(i))
		println("add", i)
		println("value", c.GetValue())
	}
}

func TestPersistCounter(t *testing.T) {
	store := NewPebbleStorage("/tmp/testx.db", pebble.Options{})
	c := NewGauge(100000)
	c.Add(10)
	time.Sleep(time.Second * 1)
	c.Add(29)
	time.Sleep(time.Second * 1)
	c.Add(30)
	c.Timestamp[1] = 1
	c.Timestamp[2] = 2
	c.Timestamp[3] = 3
	c.Timestamp[4] = 4
	c.Timestamp[5] = 5
	b, err := json.Marshal(c)
	if err != nil {
		println("error", err)
	}

	store.SetBytes("api5s", b)

	cc := NewGauge(1)

	bb, _ := store.GetBytes("api5s")

	json.Unmarshal(bb, &cc)
	fmt.Println("json", cc)
}

func TestGuageAdd(t *testing.T) {

	ctx := context.Background()
	// net.cidr_expand("1.0.0.0/1")
	// timed.Gauge.Add("api",10)
	compiler := compileModules([]string{
		`
		package test
		p { timed.Gauge.Add("","api",100,1000) > 100}
		`,
	})

	store := inmem.NewFromObject(map[string]interface{}{})
	txn := storage.NewTransactionOrDie(ctx, store)
	cancel := NewCancel()

	query := NewQuery(ast.MustParseBody("data.test.p")).
		WithCompiler(compiler).
		WithStore(store).
		WithTransaction(txn).
		WithCancel(cancel)

	go func() {
		time.Sleep(time.Millisecond * 50)
		cancel.Cancel()
	}()

	qrs, err := query.Run(ctx)

	if err != nil && err.(*Error).Code != CancelErr {
		println("Expected cancel error but got: %v (err: %v)", qrs, err)
	}
	println("result", fmt.Sprintf("%v", qrs))
}

func TestGuageDelete(t *testing.T) {

	ctx := context.Background()
	// net.cidr_expand("1.0.0.0/1")
	// timed.Gauge.Add("api",10)
	compiler := compileModules([]string{
		`
		package test
		p { timed.Gauge.Del("","api") }
		`,
	})

	store := inmem.NewFromObject(map[string]interface{}{})
	txn := storage.NewTransactionOrDie(ctx, store)
	cancel := NewCancel()

	query := NewQuery(ast.MustParseBody("data.test.p")).
		WithCompiler(compiler).
		WithStore(store).
		WithTransaction(txn).
		WithCancel(cancel)

	go func() {
		time.Sleep(time.Millisecond * 50)
		cancel.Cancel()
	}()

	qrs, err := query.Run(ctx)

	if err != nil && err.(*Error).Code != CancelErr {
		println("Expected cancel error but got: %v (err: %v)", qrs, err)
	}
}

func TestCounter(t *testing.T) {

	ctx := context.Background()
	compiler := compileModules([]string{
		`
		package test
		p {
             timed.Counter.Add("oa","api1",1) 
             timed.Counter.Add("oa","api1",2) 
             timed.Counter.Get("oa","api1") < 15
             timed.Counter.Del("oa","api1")
        }
		`,
	})

	store := inmem.NewFromObject(map[string]interface{}{})
	txn := storage.NewTransactionOrDie(ctx, store)
	cancel := NewCancel()

	query := NewQuery(ast.MustParseBody("x = data.test.p")).
		WithCompiler(compiler).
		WithStore(store).
		WithTransaction(txn)

	go func() {
		time.Sleep(time.Millisecond * 50)
		cancel.Cancel()
	}()

	qrs, err := query.Run(ctx)

	if err != nil {
		fmt.Println("error", err)
	}
	for r := range qrs {
		fmt.Println("!", r)
	}
}
