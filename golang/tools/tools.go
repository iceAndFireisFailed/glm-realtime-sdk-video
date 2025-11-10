package tools

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
)

func ConcatWavBytes(wavBytes [][]byte) ([]byte, error) {
	var combinedFrames []audio.IntBuffer
	var params *audio.Format
	var bitDepth int

	for _, wavData := range wavBytes {

		wavReader := bytes.NewReader(wavData)
		decoder := wav.NewDecoder(wavReader)

		if !decoder.IsValidFile() {
			return nil, fmt.Errorf("invalid WAV file")
		}

		buf, err := decoder.FullPCMBuffer()
		if err != nil {
			return nil, err
		}

		if params == nil {
			params = buf.Format
		} else {
			currentParams := buf.Format
			if params.SampleRate != currentParams.SampleRate ||
				params.NumChannels != currentParams.NumChannels {
				return nil, fmt.Errorf("所有 WAV 文件的参数必须相同")
			}
		}

		combinedFrames = append(combinedFrames, *buf)
		bitDepth = int(decoder.BitDepth)
	}
	if params == nil {
		return nil, fmt.Errorf("拼接音频失败，params 为空")
	}

	// 创建一个临时文件
	tempFile, err := os.CreateTemp("", "output-*.wav")
	if err != nil {
		return nil, err
	}
	defer tempFile.Close() // 确保文件会被关闭

	encoder := wav.NewEncoder(tempFile, params.SampleRate, bitDepth, params.NumChannels, 1)

	// 合并所有帧数据
	for _, buffer := range combinedFrames {
		if err := encoder.Write(&buffer); err != nil {
			return nil, err
		}
	}

	if err := encoder.Close(); err != nil {
		return nil, err
	}

	// 读取临时文件的数据到内存中
	tempFile.Seek(0, io.SeekStart)
	outputBuffer, err := io.ReadAll(tempFile)
	if err != nil {
		return nil, err
	}

	return outputBuffer, nil
}

// Pcm2Wav 将 PCM 数据转换为 WAV 格式，通过添加 WAV 文件头
// sampleRate: 采样率 (例如 16000, 44100)
// numChannels: 声道数 (1: 单声道, 2: 双声道)
// bitDepth: 位深度 (通常是 16)
func Pcm2Wav(pcmBytes []byte, sampleRate, numChannels, bitDepth int) ([]byte, error) {
	// WAV 文件头大小为 44 字节
	headerSize := 44
	fileSize := len(pcmBytes) + headerSize

	// 创建包含文件头的字节切片
	wavData := make([]byte, fileSize)

	// 1. RIFF 头
	copy(wavData[0:4], []byte("RIFF"))
	// 2. 文件大小 (文件总字节数 - 8)
	binary.LittleEndian.PutUint32(wavData[4:8], uint32(fileSize-8))
	// 3. WAVE 标记
	copy(wavData[8:12], []byte("WAVE"))
	// 4. fmt 子块
	copy(wavData[12:16], []byte("fmt "))
	// 5. fmt 子块大小 (16 表示 PCM 格式)
	binary.LittleEndian.PutUint32(wavData[16:20], 16)
	// 6. 音频格式 (1 表示 PCM)
	binary.LittleEndian.PutUint16(wavData[20:22], 1)
	// 7. 声道数
	binary.LittleEndian.PutUint16(wavData[22:24], uint16(numChannels))
	// 8. 采样率
	binary.LittleEndian.PutUint32(wavData[24:28], uint32(sampleRate))
	// 9. 字节率 (采样率 * 通道数 * 位深度 / 8)
	binary.LittleEndian.PutUint32(wavData[28:32], uint32(sampleRate*numChannels*bitDepth/8))
	// 10. 数据块对齐 (通道数 * 位深度 / 8)
	binary.LittleEndian.PutUint16(wavData[32:34], uint16(numChannels*bitDepth/8))
	// 11. 位深度
	binary.LittleEndian.PutUint16(wavData[34:36], uint16(bitDepth))
	// 12. data 子块
	copy(wavData[36:40], []byte("data"))
	// 13. 数据大小
	binary.LittleEndian.PutUint32(wavData[40:44], uint32(len(pcmBytes)))

	// 复制 PCM 数据
	copy(wavData[44:], pcmBytes)

	return wavData, nil
}

func ExtractFramesAsBase64(videoBase64 string) ([]string, error) {
	// Step 1: 解码 Base64 得到视频数据
	videoData, err := base64.StdEncoding.DecodeString(videoBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 input: %w", err)
	}

	// Step 2: 创建临时目录
	tmpDir, err := os.MkdirTemp("", "video_frames_*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func(path string) {
		err := os.RemoveAll(path)
		if err != nil {
			log.Printf("Failed to remove temp dir %s: %v", path, err)
		}
	}(tmpDir) // 函数退出后清理

	inputPath := filepath.Join(tmpDir, "input.mp4")
	outputPattern := filepath.Join(tmpDir, "frame_%04d.jpg")

	// Step 3: 写入解码后的视频数据
	if err := os.WriteFile(inputPath, videoData, 0666); err != nil {
		return nil, fmt.Errorf("failed to write input video: %w", err)
	}

	// Step 4: 调用 ffmpeg 抽帧（每秒 2 帧）
	cmd := exec.Command("ffmpeg", "-i", inputPath,
		"-vf", "fps=2", // 每秒 2 帧
		"-qscale:v", "2", // JPEG 质量（2~32，越小质量越高）
		"-f", "image2", // 输出图像序列
		outputPattern)

	// 可选：显示 ffmpeg 日志用于调试
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg failed: %w", err)
	}

	// Step 5: 查找所有生成的 JPEG 文件
	files, err := filepath.Glob(outputPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to match output files: %w", err)
	}

	var jpegBase64s []string
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			log.Printf("Failed to open %s: %v", file, err)
			continue
		}

		data, err := io.ReadAll(f)
		_ = f.Close()
		if err != nil {
			log.Printf("Failed to read %s: %v", file, err)
			continue
		}

		// 将每个 JPEG 图片编码为 Base64 字符串
		encoded := base64.StdEncoding.EncodeToString(data)
		jpegBase64s = append(jpegBase64s, encoded)
	}
	fmt.Printf("Extracted %d frames (2 FPS), encoded as base64\n", len(jpegBase64s))
	return jpegBase64s, nil
}
