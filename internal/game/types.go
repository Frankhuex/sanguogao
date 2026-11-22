package game

// Character 牌桌上的角色（与房间层 Player 解耦，仅通过 PlayerID 关联）
type Character struct {
	ID       string // 角色在本局的唯一标识
	PlayerID string // 关联房间层玩家 ID（AI 机器人可为空）
	Name     string // 开局拷贝的玩家昵称
	Seat     int    // 座次 0..N-1（逆时针顺序）

	HP       int
	MaxHP    int
	Alive    bool
	FaceDown bool // 武将牌是否背面朝上（翻面）
}

// Phase 回合内的 6 个标准阶段
type Phase int

const (
	PhasePrepare Phase = iota + 1 // 准备阶段
	PhaseJudge                    // 判定阶段
	PhaseDraw                     // 摸牌阶段
	PhasePlay                     // 出牌阶段
	PhaseDiscard                  // 弃牌阶段
	PhaseFinish                   // 结束阶段
)

var DefaultPhases = []Phase{
	PhasePrepare,
	PhaseJudge,
	PhaseDraw,
	PhasePlay,
	PhaseDiscard,
	PhaseFinish,
}

// Timing 规则集 3.0 的细分时机点（供技能与事件系统订阅）
type Timing int

const (
	TimingBeforeTurnStart Timing = iota + 1
	TimingTurnStart

	TimingPrepareStart
	TimingPrepare
	TimingPrepareEnd
	TimingBetweenPrepareAndJudge

	TimingJudgeStart
	TimingJudge
	TimingJudgeEnd
	TimingBetweenJudgeAndDraw

	TimingDrawStart
	TimingDraw
	TimingDrawEnd
	TimingBetweenDrawAndPlay

	TimingPlayStart
	TimingPlay
	TimingPlayEnd
	TimingBetweenPlayAndDiscard

	TimingDiscardStart
	TimingDiscard
	TimingDiscardEnd
	TimingBetweenDiscardAndFinish

	TimingFinishStart
	TimingFinish
	TimingFinishEnd

	TimingTurnEnd
	TimingAfterTurnEnd
)

// Turn 当前回合上下文（支持动态跳过阶段）
type Turn struct {
	Character    *Character
	Phases       []Phase
	CurrentPhase Phase
	Skipped      map[Phase]bool
}

func NewTurn(c *Character) *Turn {
	phases := make([]Phase, len(DefaultPhases))
	copy(phases, DefaultPhases)
	return &Turn{
		Character: c,
		Phases:    phases,
		Skipped:   make(map[Phase]bool),
	}
}

func (t *Turn) Skip(p Phase)           { t.Skipped[p] = true }
func (t *Turn) IsSkipped(p Phase) bool { return t.Skipped[p] }

// Game 一局游戏的运行时状态
type Game struct {
	ID       string
	RoomCode string

	Characters             []*Character          // 按座次 0..N-1 排列
	CharacterIDToCharacter map[string]*Character // 按角色 ID 快速索引
	CurrentTurn            *Turn                 // 当前回合（含当前角色与阶段）
	CurrentTiming          Timing                // 当前细分时机点
	ExtraTurns             []*Character          // 额外回合队列
}
