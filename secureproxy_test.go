package main

import (
	"testing"
)

func TestNothing(t *testing.T) {
	s := Init()

	val, err := s.handle("beer.BeerCellarService", "ListBeers", "{\"on_deck\": \"true\"}")
	if err != nil {
		t.Errorf("Bad parse: %v -> %v", val, err)
	}
}
