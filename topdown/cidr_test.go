package topdown

import (
	"context"
	"testing"
	"time"

	"github.com/meta-quick/opax/ast"
	"github.com/meta-quick/opax/storage"
	"github.com/meta-quick/opax/storage/inmem"
)

func TestNetCIDRExpandCancellation(t *testing.T) {

	ctx := context.Background()

	compiler := compileModules([]string{
		`
		package test

		p { net.cidr_expand("1.0.0.0/1") }  # generating 2**31 hosts will take a while...
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

	if err == nil || err.(*Error).Code != CancelErr {
		t.Fatalf("Expected cancel error but got: %v (err: %v)", qrs, err)
	}

}
