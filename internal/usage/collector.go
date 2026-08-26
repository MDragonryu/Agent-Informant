package usage

import "context"

type Collector interface {
	Name() string
	Collect(ctx context.Context, provider string) (Snapshot, error)
}
