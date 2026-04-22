package utilities //nolint:revive // shared utility package used across multiple modules

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"reflect"
)

const letters = 26

var runes []rune

func init() {
	for r := 'a'; r <= 'z'; r++ {
		runes = append(runes, r)
	}
	for r := '0'; r <= '9'; r++ {
		runes = append(runes, r)
	}
}

// RandomString returns a random string made up of n alpha numeric characters.
func RandomString(n int) string {
	out := make([]rune, n)
	for i := range n {
		out[i] = RandomAlphaNumeric()
	}
	return string(out)
}

// RandomLetter returns a random lowercase letter [a-z].
func RandomLetter() rune {
	return randomRune(letters)
}

func randomRune(maxIndex int) rune {
	bigInt, err := rand.Int(rand.Reader, big.NewInt(int64(maxIndex)))
	if err != nil {
		panic(fmt.Errorf("unable to generate random rune: %w", err))
	}
	return runes[bigInt.Int64()]
}

// RandomAlphaNumeric returns a random alpha numeric rune [a-z0-9].
func RandomAlphaNumeric() rune {
	return randomRune(len(runes))
}

// NewObject returns a pointer to a zero-initialized instance of the type T.
// T must be a pointer type.
func NewObject[T any]() T {
	var object T
	theType := reflect.TypeOf(object)
	if theType.Kind() != reflect.Pointer {
		panic("the generic type must be a pointer type")
	}
	theType = theType.Elem()
	return reflect.New(theType).Interface().(T)
}
