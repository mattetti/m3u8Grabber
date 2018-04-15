package m3u8

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"io"
)

// TODO: support passed IV
// TODO: maybe don't assume CBC
func aesDecrypt(src io.Reader, dst io.Writer, key []byte, iv []byte) error {
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
