package metrics

import (
	"flag"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/meta-quick/opax/test/e2e"
)

var testRuntime *e2e.TestRuntime

func TestMain(m *testing.M) {
	flag.Parse()
	testServerParams := e2e.NewAPIServerTestParams()

	var err error
	testRuntime, err = e2e.NewTestRuntimeWithOpts(e2e.TestRuntimeOpts{}, testServerParams)
	if err != nil {
		os.Exit(1)
	}

	os.Exit(testRuntime.RunTests(m))
}

func TestConcurrency(t *testing.T) {

	policy := `
	package test
	p = true
	`

	err := testRuntime.UploadPolicy(t.Name(), strings.NewReader(policy))
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup

	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go func() {
			for n := 0; n < 1000; n++ {
				dr := struct {
					Result bool `json:"result"`
				}{}
				if err := testRuntime.GetDataWithInputTyped("test/p", nil, &dr); err != nil {
					t.Error(err)
					return
				}
				if !dr.Result {
					t.Errorf("Unexpected response: %+v", dr)
					return
				}
			}
			wg.Done()
		}()
	}

	wg.Wait()

}
