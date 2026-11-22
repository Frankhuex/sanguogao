package card

import "github.com/Frankhuex/sanguogao/model"

type Card struct {
	ID       uint32
	Type     model.CardType
	Category model.CardCategory
	Suit     model.CardSuit
	Number   int32 // 1..13 (A..K)
}

func (c *Card) ToProto() *model.CardInfo {
	if c == nil {
		return nil
	}
	return &model.CardInfo{
		Id:       c.ID,
		Type:     c.Type,
		Category: c.Category,
		Suit:     c.Suit,
		Number:   c.Number,
	}
}

func CategoryOf(t model.CardType) model.CardCategory {
	switch t {
	case model.CardType_CARD_TYPE_SHA,
		model.CardType_CARD_TYPE_SHAN,
		model.CardType_CARD_TYPE_TAO:
		return model.CardCategory_CARD_CATEGORY_BASIC
	default:
		return model.CardCategory_CARD_CATEGORY_UNSPECIFIED
	}
}

func TypeName(t model.CardType) string {
	switch t {
	case model.CardType_CARD_TYPE_SHA:
		return "杀"
	case model.CardType_CARD_TYPE_SHAN:
		return "闪"
	case model.CardType_CARD_TYPE_TAO:
		return "桃"
	default:
		return "未知牌"
	}
}

func ToProtoSlice(cards []*Card) []*model.CardInfo {
	res := make([]*model.CardInfo, len(cards))
	for i, c := range cards {
		res[i] = c.ToProto()
	}
	return res
}
