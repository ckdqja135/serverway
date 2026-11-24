package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const sumFile = "../../../web/src/wasm/source.sum"

// 화면에 넣은 game.wasm이 지금 규칙 코드로 만든 것인지 확인한다.
func TestWasmIsFresh(t *testing.T) {
	files := []string{"main.go", "../../go.mod", "../../internal/sim/balance.json"}
	for _, dir := range []string{"../../internal/sim", "../../internal/game"} {
		list, _ := filepath.Glob(dir + "/*.go")
		for _, f := range list {
			if !strings.HasSuffix(f, "_test.go") && !strings.HasSuffix(f, "http.go") {
				files = append(files, f)
			}
		}
	}
	sort.Strings(files)
	h := sha256.New()
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(h, "%s %d\n%s", f, len(b), b)
	}
	sum := fmt.Sprintf("%x\n", h.Sum(nil))
	if os.Getenv("WASM_SUM") == "write" {
		if err := os.WriteFile(sumFile, []byte(sum), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if old, _ := os.ReadFile(sumFile); string(old) != sum {
		t.Fatal("규칙 코드가 바뀌었습니다. scripts/wasm.sh 를 실행하세요")
	}
}
