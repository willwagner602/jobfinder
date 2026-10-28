package main

import (
	"context"
	"fmt"
	"jobfinder/internal/db"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		panic("usage: migrate-careers DB_PATH")
	}
	d, e := db.Open(os.Args[1])
	if e != nil {
		panic(e)
	}
	defer d.Close()
	if e = db.EnsureCurrent(context.Background(), d); e != nil {
		panic(e)
	}
	fmt.Println("migration complete")
}
