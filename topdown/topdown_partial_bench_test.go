package topdown

import (
	"context"
	"fmt"
	"testing"

	"github.com/meta-quick/opax/ast"
	"github.com/meta-quick/opax/storage"
	"github.com/meta-quick/opax/storage/inmem"
)

func BenchmarkInliningFullScan(b *testing.B) {

	ctx := context.Background()
	body := ast.MustParseBody("data.test.p = true")
	unknowns := []*ast.Term{ast.MustParseTerm("input")}
	compiler := ast.MustCompileModules(map[string]string{
		"test.rego": `
		package test

		p {
			data.a[i] == input
		}
		`,
	})

	sizes := []int{1000, 10000, 300000}

	for _, n := range sizes {

		b.Run(fmt.Sprint(n), func(b *testing.B) {

			store := inmem.NewFromObject(generateInlineFullScanBenchmarkData(n))

			b.ResetTimer()

			for i := 0; i < b.N; i++ {

				err := storage.Txn(ctx, store, storage.TransactionParams{}, func(txn storage.Transaction) error {

					q := NewQuery(body).
						WithCompiler(compiler).
						WithStore(store).
						WithTransaction(txn).
						WithUnknowns(unknowns)

					queries, support, err := q.PartialRun(ctx)
					if err != nil {
						b.Fatal(err)
					}

					if len(queries) != n {
						b.Fatal("Expected", n, "queries")
					} else if len(support) != 0 {
						b.Fatal("Unexpected support")
					}

					return nil
				})
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}

}

func generateInlineFullScanBenchmarkData(n int) map[string]interface{} {

	sl := make([]interface{}, n)
	for i := range sl {
		sl[i] = fmt.Sprint(i)
	}

	return map[string]interface{}{
		"a": sl,
	}
}
