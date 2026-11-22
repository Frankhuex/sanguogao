package card

import (
	"math/rand/v2"

	"github.com/Frankhuex/sanguogao/model"
)

type Deck struct {
	drawPile    []*Card
	discardPile []*Card
}

// NewDefaultDeck 创建 V1 默认基本牌堆（54张：30杀、15闪、9桃）并洗牌
func NewDefaultDeck() *Deck {
	cards := make([]*Card, 0, 54)
	var id uint32 = 1

	addCards := func(cardType model.CardType, suits []model.CardSuit, count int) {
		cat := CategoryOf(cardType)
		for i := range count {
			cards = append(cards, &Card{
				ID:       id,
				Type:     cardType,
				Category: cat,
				Suit:     suits[i%len(suits)],
				Number:   int32(i%13) + 1,
			})
			id++
		}
	}

	addCards(model.CardType_CARD_TYPE_SHA, []model.CardSuit{
		model.CardSuit_CARD_SUIT_SPADE,
		model.CardSuit_CARD_SUIT_CLUB,
		model.CardSuit_CARD_SUIT_HEART,
		model.CardSuit_CARD_SUIT_DIAMOND,
	}, 30)
	addCards(model.CardType_CARD_TYPE_SHAN, []model.CardSuit{
		model.CardSuit_CARD_SUIT_DIAMOND,
		model.CardSuit_CARD_SUIT_HEART,
	}, 15)
	addCards(model.CardType_CARD_TYPE_TAO, []model.CardSuit{
		model.CardSuit_CARD_SUIT_HEART,
		model.CardSuit_CARD_SUIT_DIAMOND,
	}, 9)

	d := &Deck{drawPile: cards}
	d.Shuffle()
	return d
}

// NewDeckWithCards 使用指定顺序的牌创建牌堆（用于确定性测试）
func NewDeckWithCards(cards []*Card) *Deck {
	copied := make([]*Card, len(cards))
	copy(copied, cards)
	return &Deck{drawPile: copied}
}

func (d *Deck) Shuffle() {
	rand.Shuffle(len(d.drawPile), func(i, j int) {
		d.drawPile[i], d.drawPile[j] = d.drawPile[j], d.drawPile[i]
	})
}

// Draw 摸 n 张牌；牌堆不足时自动将弃牌堆洗牌补充
func (d *Deck) Draw(n int) []*Card {
	res := make([]*Card, 0, n)
	for range n {
		if len(d.drawPile) == 0 {
			if len(d.discardPile) == 0 {
				break
			}
			d.drawPile = append(d.drawPile, d.discardPile...)
			d.discardPile = d.discardPile[:0]
			d.Shuffle()
		}
		top := d.drawPile[0]
		d.drawPile = d.drawPile[1:]
		res = append(res, top)
	}
	return res
}

func (d *Deck) Discard(cards ...*Card) {
	d.discardPile = append(d.discardPile, cards...)
}

func (d *Deck) DrawCount() int    { return len(d.drawPile) }
func (d *Deck) DiscardCount() int { return len(d.discardPile) }
