// playtest: 화면 없이 봇 전략별 결과를 비교한다. 밸런스를 바꾼 뒤 확인할 때 쓴다.
// 기록 파일(/api/sessions/{id}/replay 응답)을 인자로 주면 그 판을 다시 계산하고, hard를 주면 하드 모드로 비교한다.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"serverway/internal/sim"
)

func replay(path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	var in struct {
		Seed  uint64         `json:"seed"`
		Rules int            `json:"rules"`
		Hard  bool           `json:"hard"`
		Tick  int            `json:"tick"`
		Score int            `json:"score"`
		Log   []sim.LogEntry `json:"log"`
	}
	if err := json.Unmarshal(b, &in); err != nil {
		log.Fatal(err)
	}
	base := sim.Default()
	if in.Hard {
		base = sim.Hard(base)
	}
	cfg, err := sim.ForRules(base, in.Rules)
	if err != nil {
		log.Fatal(err)
	}
	s := sim.Replay(cfg, in.Seed, in.Log, in.Tick)
	fmt.Printf("기록 %d점, 다시 계산 %d점, %d틱, 실패 %v, 일치 %v\n", in.Score, s.Score, s.Tick, s.Failures, in.Score == s.Score && in.Tick == s.Tick)
}

func main() {
	cfg := sim.Default()
	hard := len(os.Args) > 1 && os.Args[1] == "hard"
	if len(os.Args) > 1 && !hard {
		replay(os.Args[1])
		return
	}
	bots := []struct {
		name string
		b    sim.Bot
	}{
		{"아무것도 안 함", sim.Bot{}},
		{"연결만", sim.Bot{Link: true}},
		{"서버만 증설", sim.Bot{Link: true, API: true, LinkUp: true, Reroute: true}},
		{"DB만 증설", sim.Bot{Link: true, DB: true}},
		{"링크 개선 없음", sim.Bot{Link: true, API: true, DB: true, Reroute: true}},
		{"우회로 없음", sim.Bot{Link: true, API: true, DB: true, LinkUp: true}},
		{"균형", sim.Bot{Link: true, API: true, DB: true, LinkUp: true, Reroute: true}},
		{"균형과 장애 대비", sim.Bot{Link: true, API: true, DB: true, LinkUp: true, Reroute: true, Prepare: true}},
		{"균형과 저장 공간", sim.Bot{Link: true, API: true, DB: true, LinkUp: true, Reroute: true, Storage: true}},
		{"저장 공간과 Redis", sim.Bot{Link: true, API: true, DB: true, LinkUp: true, Reroute: true, Storage: true, Redis: true}},
		{"전부", sim.Bot{Link: true, API: true, DB: true, LinkUp: true, Reroute: true, Storage: true, Redis: true, WS: true}},
	}
	if hard {
		cfg = sim.Hard(cfg)
		bots = append(bots[len(bots)-3:], struct {
			name string
			b    sim.Bot
		}{"전부와 보안 패치", sim.Bot{Link: true, API: true, DB: true, LinkUp: true, Reroute: true, Storage: true, Redis: true, WS: true, Patch: true}})
	}
	for _, x := range bots {
		for seed := uint64(1); seed <= 5; seed++ {
			b := x.b
			s := sim.Play(cfg, seed, &b, 3600)
			servers, levels := 0, 0
			for _, n := range s.Nodes {
				if n.Kind != sim.Source {
					servers++
					levels += n.Level + 1
				}
			}
			fmt.Printf("%-16s 시드 %d  %-5s %4d초  완료 %4d  실패 %v  상태 %3d%%  남은 예산 %d  서버 %d대 레벨 합 %d 링크 %d 최고 %.1f/s\n",
				x.name, seed, s.Over, s.Tick/sim.TPS, s.Completed, s.Failures, s.Health*100/s.Cfg.Health.Max, s.Budget, servers, levels, len(s.Links), float64(s.PeakRPS)/10)
		}
	}
}
