package alarm

import "testing"

// TestSendTwilio_ShortAccountSIDDoesNotPanic checks that a Twilio account SID shorter
// than 10 characters (a typo or a truncated env var) does not panic in the debug log
// line that prints its prefix. No recipients are given, so no HTTP request is made.
func TestSendTwilio_ShortAccountSIDDoesNotPanic(t *testing.T) {
	n := &SMSNotifier{config: &SMSGlobalConfig{AccountSID: "AC12", AuthToken: "tok", FromNumber: "+15550000000"}}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("sendTwilio panicked on a short SID: %v", r)
		}
	}()
	if err := n.sendTwilio(&SMSConfig{}, "hello"); err != nil {
		t.Fatalf("unexpected error with no recipients: %v", err)
	}
}

// TestMaskString_ShortInputs checks maskString on inputs too short to show a suffix;
// a one-character secret used to panic with a negative slice index.
func TestMaskString_ShortInputs(t *testing.T) {
	cases := map[string]string{
		"":           "(not set)",
		"a":          "***",
		"ab":         "***",
		"abc":        "***bc",
		"abcdefgh":   "***gh",
		"abcdefghij": "abcd****ghij",
	}
	for in, want := range cases {
		if got := maskString(in); got != want {
			t.Errorf("maskString(%q) = %q, want %q", in, got, want)
		}
	}
}
