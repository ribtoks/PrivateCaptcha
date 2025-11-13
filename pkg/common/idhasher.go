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
	saltValue := salt.Value()
	if len(saltValue) == 0 {
		return &idHasher{}
	}

	data := hashids.NewData()
	data.Salt = saltValue
	data.MinLength = 10

	return &idHasher{
		hashIDData: data,
	}
}

func (ih *idHasher) Encrypt(id int) string {
	if ih.hashIDData != nil {
		if h, err := hashids.NewWithData(ih.hashIDData); err == nil {
			if e, err := h.Encode([]int{id}); err == nil {
				return e
			}
		}
	}

	return strconv.Itoa(int(id))
}

func (ih *idHasher) Decrypt(hash string) (int, error) {
	if ih.hashIDData == nil {
		return strconv.Atoi(hash)
	}

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
