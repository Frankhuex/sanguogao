package game

// WinCondition 胜负判定接口（默认最后存活者胜，后续可替换为身份局规则）
type WinCondition interface {
	Check(players []*GamePlayer) (over bool, winner *GamePlayer)
}

type LastStandingWinCondition struct{}

func (LastStandingWinCondition) Check(players []*GamePlayer) (bool, *GamePlayer) {
	var alive []*GamePlayer
	for _, p := range players {
		if p.Alive {
			alive = append(alive, p)
		}
	}
	if len(alive) == 1 {
		return true, alive[0]
	}
	if len(alive) == 0 {
		return true, nil
	}
	return false, nil
}
