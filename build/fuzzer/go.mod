module github.com/meta-quick/fuzz-opa

go 1.15

require (
	github.com/dvyukov/go-fuzz v0.0.0-20210429054444-fca39067bc72 // indirect
	github.com/meta-quick/opax v0.0.0
)

// Point the OPA dependency to the local source
replace github.com/meta-quick/opax => ../../
