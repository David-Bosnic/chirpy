package main

import "testing"

func TestCleanBody(t *testing.T) {
	type values struct {
		input  string
		output string
	}
	cases := []values{
		{
			input:  "Hello I'm bob",
			output: "Hello I'm bob",
		},
		{
			input:  "Pain",
			output: "Pain",
		},
		{
			input:  "I love a good kerfuffle",
			output: "I love a good ****",
		},
		{
			input:  "I love a good Kerfuffle",
			output: "I love a good ****",
		},
		{
			input:  "kerfuffle I barly even know her",
			output: "**** I barly even know her",
		},
		{
			input:  "sharbert I barly even know her",
			output: "**** I barly even know her",
		},
		{
			input:  "fornax I barly even know her",
			output: "**** I barly even know her",
		},
		{
			input:  "fornaxing I barly even know her",
			output: "****ing I barly even know her",
		},
		{
			input:  "kerFuffle sharBERT forNaX",
			output: "**** **** ****",
		},
	}
	for _, val := range cases {
		cleanedTxt := cleanBody(val.input)
		if cleanedTxt != val.output {
			t.Errorf("Output did not match input. \nGot:%s \nExp:%s\n", cleanedTxt, val.output)
		}
	}
}
