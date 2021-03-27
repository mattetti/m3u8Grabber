package m3u8

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/asticode/go-astits"
)

// TODO: support passed IV
// TODO: maybe don't assume CBC
func aesDecrypt(src io.Reader, dst io.Writer, j *WJob) error {
	key, iv := j.Key, j.IV
	if j.Crypto == "sample-aes" {
		return fmt.Errorf("this isn't an aes-128 encrypted file but sample-aes (aka Apple's FairPlay DRM)")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("failed to use the passed key - %v", err)
	}

	// IV
	if len(iv) == 0 {
		iv = make([]byte, aes.BlockSize)
	}
	if len(iv) != aes.BlockSize {
		return fmt.Errorf("bad IV, should be len %d but is %d", aes.BlockSize, len(iv))
	}
	if Debug {
		Logger.Println("About to decrypt with IV:", iv)
	}

	mode := cipher.NewCBCDecrypter(block, iv)

	// 2MB buffer
	buf := make([]byte, 2000000)
	var n int
	for err == nil {
		n, err = src.Read(buf)
		if n == 0 || (err != nil && err != io.EOF) {
			break
		}
		if n < aes.BlockSize || (n%aes.BlockSize) != 0 {
			Logger.Printf("We hit a weirdly size chunk, someone should take a look - size: %d", n)
			break
		}

		mode.CryptBlocks(buf[:n], buf[:n])
		n, err = dst.Write(buf[:n])
	}
	if err == io.EOF {
		return nil
	}
	return err
}

type AESAudioSetupInformation struct {
	audio_type        [4]byte // 4 bytes
	priming           [2]byte // 2 bytes
	version           byte    // 1 byte
	setup_data_length byte    // 1 byte
	// setup_data               // setup_data_length
}

// https://developer.apple.com/library/archive/documentation/AudioVideo/Conceptual/HLS_Sample_Encryption/TransportStreamSignaling/TransportStreamSignaling.html#//apple_ref/doc/uid/TP40012862-CH3-SW1
var pmtStreamType = map[uint8]string{
	0xdb: "H.264-Video",
	0xcf: "AAC-Audio",
	0xc1: "AC-3-Audio",
	0xc2: "Enhanced-AC-3-Audio",
}

// https://developer.apple.com/library/archive/documentation/AudioVideo/Conceptual/HLS_Sample_Encryption/Intro/Intro.html#//apple_ref/doc/uid/TP40012862-CH5-SW1
func sampleAESdecrypt(src *os.File, dst io.Writer, j *WJob) error {
	// Not currently working, we need to extract each frame and decrypt them
	// accordinding to the protocol.
	// And then remux the stream.

	ctx, _ := context.WithCancel(context.Background())
	// demuxer
	dmx := astits.NewDemuxer(ctx, src)
	packets := []*astits.PESData{}
	for {
		d, err := dmx.NextData()
		if err != nil {
			if err != astits.ErrNoMorePackets {
				return err
			}
			break
		}

		if d.PES != nil {
			// if AAC => https://wiki.multimedia.cx/index.php/ADTS
			fmt.Printf("Stream ID: %d\n", d.PES.Header.StreamID)
			fmt.Printf("PES packet length: %d\n", d.PES.Header.PacketLength)
			// fmt.Printf("Partial payload\n: %+X", d.PES.Data[:16])
			packets = append(packets, d.PES)
		}

		if d.PMT != nil {
			// Loop through elementary streams
			for _, es := range d.PMT.ElementaryStreams {
				fmt.Printf("Stream detected, type: %s\n", pmtStreamType[uint8(es.StreamType)])
				if pmtStreamType[uint8(es.StreamType)] == "AAC-Audio" {
					// https://wiki.multimedia.cx/index.php/ADTS
					fmt.Println("ADTS stream stream")
				}
			}
		}
	}

	_, err := src.Seek(0, 0)
	if err != nil {
		log.Fatal("failed to rewind the source")
	}
	n, err := io.Copy(dst, src)
	fmt.Println(n, err)

	return nil
}
