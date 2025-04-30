package auth

import (
	"github.com/google/uuid"
	"testing"
	"time"
)

type baseInput struct {
	userID      uuid.UUID
	tokenSecret string
	duration    time.Duration
}
type baseOutput struct {
	userID uuid.UUID
}
type values struct {
	input  baseInput
	output baseOutput
}

func TestJWTCreation(t *testing.T) {
	testID, err := uuid.NewUUID()
	if err != nil {
		t.Error("Error generating newUUID:", err)
		return
	}
	cases := []values{
		{
			input: baseInput{
				userID:      testID,
				tokenSecret: "Spooky",
				duration:    time.Second * 5,
			},
			output: baseOutput{
				userID: testID,
			},
		},
		{
			input: baseInput{
				userID:      testID,
				tokenSecret: "Scary",
				duration:    time.Second * 10,
			},
			output: baseOutput{
				userID: testID,
			},
		},
		{
			input: baseInput{
				userID:      testID,
				tokenSecret: "Skeletons",
				duration:    time.Second * 1,
			},
			output: baseOutput{
				userID: testID,
			},
		},
	}
	for i, val := range cases {
		input := val.input
		output := val.output
		token, err := MakeJWT(input.userID, input.tokenSecret, input.duration)
		if err != nil {
			t.Errorf("Case %d: Failed to create a JWT: %s", i, err)
			continue
		}
		uuid, err := ValidateJWT(token, input.tokenSecret)
		if err != nil {
			t.Errorf("Case %d: Failed to validate JWT: %s", i, err)
		} else if uuid != output.userID {
			t.Errorf("Case %d: UUID of output does not match input", i)
		}

		if input.duration <= time.Second {
			time.Sleep(input.duration + time.Second)
			_, err := ValidateJWT(token, input.tokenSecret)
			if err == nil {
				t.Errorf("Case %d: Token did not expire as expected", i)
			} else {
				t.Logf("Case %d: Token expired as expected: %s", i, err)
			}
		}
	}
}
