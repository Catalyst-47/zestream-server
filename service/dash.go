package service

import (
	"bytes"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"zestream-server/constants"
	"zestream-server/utils"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)

func GenerateDash(fileName string, watermarkFileName string, watermarkFileDimension map[string]int, watermarkPosition map[string]int, withWatermark bool) {

	targetFile, err := utils.GetDownloadFilePathName(fileName)
	if err != nil {
		log.Println(err)
	}

	watermarkFile, err := utils.GetDownloadFilePathName(watermarkFileName)
	if err != nil {
		log.Println(err)
	}

	var fileNameStripped = utils.RemoveExtensionFromFile(fileName)

	outputPath, err := utils.GetOutputFilePathName(fileName, fileNameStripped)
	if err != nil {
		log.Println(err)
		return
	}

	var wg sync.WaitGroup

	wg.Add(len(constants.AudioFileTypeMap) + len(constants.VideoFileTypeMap) + 1)

	generateAudioFiles(targetFile, outputPath, &wg)

	generateVideoFiles(targetFile, outputPath, &wg)

	generateThumbnailFiles(targetFile, outputPath, &wg)

	wg.Wait()

	generateMPD(outputPath)
}

func generateAudioFiles(targetFile string, outputPath string, wg *sync.WaitGroup) {
	for fileType, filePrefix := range constants.AudioFileTypeMap {
		var outputFile = outputPath + filePrefix

		go generateMultiBitrateAudio(targetFile, outputFile, fileType, wg)
	}
}

func generateVideoFiles(targetFile string, outputPath string, wg *sync.WaitGroup) {
	for fileType, filePrefix := range constants.VideoFileTypeMap {
		var outputFile = outputPath + filePrefix

		go generateMultiBitrateVideo(targetFile, outputFile, fileType, wg)
	}
}

func generateThumbnailFiles(targetFile string, outputPath string, wg *sync.WaitGroup) {
	for _, filePrefix := range constants.ImageFileTypeMap {
		var outputFile = outputPath + filePrefix

		go generateThumbnails(targetFile, outputFile, constants.DEFAULT_THUMBNAIL_TIMESTAMP, wg)
	}
}

func generateMultiBitrateAudio(targetFile string, outputFile string, fileType constants.FILE_TYPE, wg *sync.WaitGroup) {
	ffmpeg.Input(targetFile, ffmpeg.KwArgs{
		constants.AudioKwargs[constants.HWAccel]: constants.FFmpegConfig[constants.HWAccel],
	}).
		Output(outputFile, ffmpeg.KwArgs{
			constants.AudioKwargs[constants.AudioCodec]:        constants.FFmpegConfig[constants.AudioCodec],
			constants.AudioKwargs[constants.AudioBitrate]:      constants.AudioBitrateMap[fileType],
			constants.AudioKwargs[constants.AllowSoftEncoding]: constants.FFmpegConfig[constants.AllowSoftEncoding],
			constants.AudioKwargs[constants.VideoExclusion]:    constants.FFmpegConfig[constants.VideoExclusion],
		}).
		OverWriteOutput().ErrorToStdOut().Run()

	wg.Done()
}

func generateCappedBitrateVideo(targetFile string, outputFile string, fileType constants.FILE_TYPE, wg *sync.WaitGroup) {

	ffmpeg.Input(targetFile).Output(outputFile, ffmpeg.KwArgs{
		constants.VideoKwargs[constants.Preset]:         constants.FFmpegConfig[constants.Preset],
		constants.VideoKwargs[constants.Tune]:           constants.FFmpegConfig[constants.Tune],
		constants.VideoKwargs[constants.FpsMode]:        constants.FFmpegConfig[constants.FpsMode],
		constants.VideoKwargs[constants.AudioExclusion]: constants.FFmpegConfig[constants.AudioExclusion],
		constants.VideoKwargs[constants.VideoCodec]:     constants.FFmpegConfig[constants.VideoCodec],
		constants.VideoKwargs[constants.MaxRate]:        constants.VideoBitrateMap[fileType],
		constants.VideoKwargs[constants.BufferSize]:     constants.VideoBufferSizeMap[fileType],
		constants.VideoKwargs[constants.VideoFormat]:    constants.FFmpegConfig[constants.VideoFormat],
	}).
		OverWriteOutput().
		ErrorToStdOut().
		Run()

	wg.Done()
}

func getOverlay(waterMarkFile string, watermarkFileDimension map[string]int) *ffmpeg.Stream {
	overlayArgs := "" + strconv.Itoa(watermarkFileDimension[constants.WaterMarkSizeMap[constants.X]]) + ":" + strconv.Itoa(watermarkFileDimension[constants.WaterMarkSizeMap[constants.Y]]) + ""
	return ffmpeg.Input(waterMarkFile).Filter(constants.Scale, ffmpeg.Args{overlayArgs})
}

func generateCappedBitrateVideoWithWaterMark(targetFile string, outputFile string, fileType constants.FILE_TYPE, waterMarkFile string, watermarkFileDimension map[string]int, watermarkPosition map[string]int, wg *sync.WaitGroup) {
	filterArgs := "" + strconv.Itoa(watermarkPosition[constants.WaterMarkPositionMap[constants.X]]) + ":" + strconv.Itoa(watermarkPosition[constants.WaterMarkPositionMap[constants.Y]]) + ""

	ffmpeg.Filter(
		[]*ffmpeg.Stream{
			ffmpeg.Input(targetFile),
			getOverlay(waterMarkFile, watermarkFileDimension),
		}, constants.Overlay, ffmpeg.Args{filterArgs}).
		Output(outputFile, ffmpeg.KwArgs{
			constants.VideoKwargs[constants.Preset]:         constants.FFmpegConfig[constants.Preset],
			constants.VideoKwargs[constants.Tune]:           constants.FFmpegConfig[constants.Tune],
			constants.VideoKwargs[constants.FpsMode]:        constants.FFmpegConfig[constants.FpsMode],
			constants.VideoKwargs[constants.AudioExclusion]: constants.FFmpegConfig[constants.AudioExclusion],
			constants.VideoKwargs[constants.VideoCodec]:     constants.FFmpegConfig[constants.VideoCodec],
			constants.VideoKwargs[constants.MaxRate]:        constants.VideoBitrateMap[fileType],
			constants.VideoKwargs[constants.BufferSize]:     constants.VideoBufferSizeMap[fileType],
			constants.VideoKwargs[constants.VideoFormat]:    constants.FFmpegConfig[constants.VideoFormat],
		}).
		OverWriteOutput().
		ErrorToStdOut().
		Run()

	wg.Done()
}

// generateThumbnail generates a thumbnail at given timestamp, from the target file and write it to output file
func generateThumbnails(targetFile string, outputFile string, timeStamp string, wg *sync.WaitGroup) {
	ffmpeg.Input(targetFile).
		Output(outputFile, ffmpeg.KwArgs{
			constants.VideoKwargs[constants.ScreenShot]:  timeStamp,
			constants.VideoKwargs[constants.VideoFrames]: constants.FFmpegConfig[constants.VideoFrames],
		}).
		OverWriteOutput().
		ErrorToStdOut().
		Run()

	wg.Done()
}

// generateMPD (Media Presentation Description), generates the XML description file
// for the given output path.
func generateMPD(outputPath string) {
	var fileArgs bytes.Buffer

	checkFileExistsAndAppendToBuffer(&fileArgs, outputPath, constants.AudioFileTypeMap)
	checkFileExistsAndAppendToBuffer(&fileArgs, outputPath, constants.VideoFileTypeMap)

	var filePaths = strings.TrimSuffix(fileArgs.String(), " ")

	var inputArgsMap = map[string]string{
		constants.Mp4BoxArgs[constants.Dash]:        constants.Mp4BoxConfig[constants.Dash],
		constants.Mp4BoxArgs[constants.Rap]:         constants.Mp4BoxConfig[constants.Rap],
		constants.Mp4BoxArgs[constants.FragRap]:     constants.Mp4BoxConfig[constants.FragRap],
		constants.Mp4BoxArgs[constants.BsSwitching]: constants.Mp4BoxConfig[constants.BsSwitching],
		constants.Mp4BoxArgs[constants.Profile]:     constants.Mp4BoxConfig[constants.Profile],
		constants.Mp4BoxArgs[constants.Out]:         outputPath + constants.DashOutputExt,
	}

	inputArgsStr := utils.StringToArgsGenerator(inputArgsMap)

	var argsArr = strings.Split(inputArgsStr+filePaths, " ")

	cmd := exec.Command(constants.MP4Box, argsArr...)

	o, err := cmd.CombinedOutput()

	if err != nil {
		log.Println(err)
	}

	err = utils.DeleteFiles(filePaths)

	if err != nil {
		log.Println(err)
	}

	log.Println(string(o))
}

// checkFileExistsAndAppendToBuffer checks if the given output file exits, then appends the
// path to buffer.
func checkFileExistsAndAppendToBuffer(fileArgs *bytes.Buffer, outputPath string, fileTypes map[constants.FILE_TYPE]string) {
	for _, filePrefix := range fileTypes {
		var outputFile = outputPath + filePrefix
		if utils.IsFileValid(outputFile) {
			fileArgs.WriteString(utils.WrapStringInQuotes(outputFile))
		}
	}
}
