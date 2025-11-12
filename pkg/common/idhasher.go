package common

import (
	"errors"
	"strconv"

	"github.com/speps/go-hashids/v2"
)

type idHasher struct {
	hashIDData *hashids.HashIDData
}

var errUnexpectedIdentifierLen = errors.New("unexpected identifier length")

var _ IdentifierHasher = (*idHasher)(nil)

func NewIDHasher(salt ConfigItem) IdentifierHasher {
	data := hashids.NewData()
	data.Salt = salt.Value()
	data.MinLength = 10

	return &idHasher{
		hashIDData: data,
	}
}

func (ih *idHasher) Encrypt(id int) string {
	h, err := hashids.NewWithData(ih.hashIDData)
	if err != nil {
		return strconv.Itoa(int(id))
	}
	e, err := h.Encode([]int{id})
	if err != nil {
		return strconv.Itoa(int(id))
	}
	return e
}

func (ih *idHasher) Decrypt(hash string) (int, error) {
	h, err := hashids.NewWithData(ih.hashIDData)
	if err != nil {
		return -1, err
	}

	d, err := h.DecodeWithError(hash)
	if err != nil {
		return -1, err
	}

	if len(d) != 1 {
		return -1, errUnexpectedIdentifierLen
	}

	return d[0], nil
}
