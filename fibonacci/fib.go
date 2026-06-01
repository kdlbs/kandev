package main

import (
	"fmt"
	"os"
	"strconv"
)

func Fib(n int) (uint64, error) {
	if n < 0 {
		return 0, fmt.Errorf("n must be non-negative")
	}
	var a, b uint64 = 0, 1
	for i := 0; i < n; i++ {
		a, b = b, a+b
	}
	return a, nil
}

func main() {
	n := 10
	if len(os.Args) > 1 {
		v, err := strconv.Atoi(os.Args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "invalid n:", err)
			os.Exit(1)
		}
		n = v
	}
	r, err := Fib(n)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(r)
}
