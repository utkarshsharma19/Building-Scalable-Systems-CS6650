package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const lines = 100_000

func unbuffered(path string) time.Duration {
	f, _ := os.Create(path)
	start := time.Now()
	for i := range lines {
		f.Write([]byte(fmt.Sprintf("line %d\n", i)))
	}
	f.Close()
	return time.Since(start)
}

func buffered(path string) time.Duration {
	f, _ := os.Create(path)
	w := bufio.NewWriter(f)
	start := time.Now()
	for i := range lines {
		w.WriteString(fmt.Sprintf("line %d\n", i))
	}
	w.Flush()
	f.Close()
	return time.Since(start)
}

func main() {
	path := filepath.Join(os.TempDir(), "fileaccess.txt")
	defer os.Remove(path)

	var ut, bt time.Duration
	for r := 1; r <= 3; r++ {
		u, b := unbuffered(path), buffered(path)
		ut, bt = ut+u, bt+b
		fmt.Printf("run %d: unbuffered=%v  buffered=%v  (%.1fx)\n", r, u, b, float64(u)/float64(b))
	}
	fmt.Printf("\nMEAN (%d lines): unbuffered=%v  buffered=%v  speedup=%.1fx\n",
		lines, ut/3, bt/3, float64(ut)/float64(bt))
}
