package main

import "context"

type application interface {
	Run(ctx context.Context) error
}
