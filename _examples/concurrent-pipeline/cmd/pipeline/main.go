// Command pipeline runs a small concurrent pipeline and then exercises a few
// defect paths that Sheriff reports.
package main

import (
	"fmt"

	"example/pipeline/internal/pipeline"
)

func main() {
	// The concurrent pipeline itself runs safely.
	agg := &pipeline.Aggregator{}
	p := pipeline.New(agg)
	p.Start(2)
	p.Submit(1)
	p.Stop()

	// Each of the following triggers a defect Sheriff reports.
	fmt.Println(pipeline.MustDescribe())
	fmt.Println(pipeline.Sample())
	fmt.Println(pipeline.RunHandler())
}
