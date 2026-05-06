package tesedi

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDate_UnmarshalJSON(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    time.Time
		isZero  bool
		wantErr bool
	}{
		{name: "valid_yyyy_mm_dd", input: `"2026-04-30"`, want: time.Date(2026, time.April, 30, 0, 0, 0, 0, time.UTC)},
		{name: "empty_string_is_zero", input: `""`, isZero: true},
		{name: "json_null_is_zero", input: `null`, isZero: true},
		{name: "with_time_component_rejected", input: `"2026-04-30T10:00:00"`, wantErr: true},
		{name: "slashes_rejected", input: `"2026/04/30"`, wantErr: true},
		{name: "junk_rejected", input: `"not a date"`, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var d Date
			err := json.Unmarshal([]byte(tc.input), &d)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tc.isZero {
				assert.True(t, d.IsZero())
				return
			}
			assert.True(t, tc.want.Equal(d.Time), "got %s, want %s", d.Time, tc.want)
		})
	}
}

func TestDate_MarshalJSON(t *testing.T) {
	t.Run("populated_date_round_trips", func(t *testing.T) {
		d := Date{Time: time.Date(2026, time.April, 30, 0, 0, 0, 0, time.UTC)}
		out, err := json.Marshal(d)
		require.NoError(t, err)
		assert.Equal(t, `"2026-04-30"`, string(out))
	})

	t.Run("zero_emits_empty_string", func(t *testing.T) {
		var d Date
		out, err := json.Marshal(d)
		require.NoError(t, err)
		assert.Equal(t, `""`, string(out))
	})
}
