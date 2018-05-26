package m3u8_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/mattetti/m3u8Grabber/m3u8"
)

func TestExtractRendition(t *testing.T) {
	tests := []struct {
		l    string
		want m3u8.Rendition
	}{
		{
			l: `#EXT-X-STREAM-INF:PROGRAM-ID=1,BANDWIDTH=622000,RESOLUTION=512x288,CODECS="avc1.66.30, mp4a.40.2",CLOSED-CAPTIONS=NONE`,
			want: m3u8.Rendition{
				ProgramID:  1,
				Bandwidth:  622000,
				Resolution: "512x288",
				Codecs:     []string{"avc1.66.30", "mp4a.40.2"},
			},
		},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprintf("test %d", i), func(t *testing.T) {
			if got := m3u8.ExtractRendition(tt.l); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractRendition() = %v, want %v", got, tt.want)
			}
		})
	}
}
