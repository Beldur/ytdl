package main

import (
    "ytlib"
    "fmt"
    "crypto/sha1"
    "rpctypes"
    "io"
    "path"
    "path/filepath"
    "os"
    "strconv"
    "os/exec"
)

// Video Status options
const (
    DOWNLOADING rpctypes.VideoProcessStatus = iota
    CONVERTING
    DONE
    ERROR
)

type DownloadManager struct {
    downloadDir string
    queueCounter int
    videoList map[string]*rpctypes.VideoStatus
}

func (this *DownloadManager) Init(downloadDirectory string) *DownloadManager {
    return &DownloadManager {
        downloadDirectory,
        0,
        map[string]*rpctypes.VideoStatus {},
    }
}

// Request a new video for download
func (this *DownloadManager) StartDownload(videoId string, options ytlib.DownloadOptions) (*rpctypes.VideoStatus, error) {
    // Do we have a enough for our guest?
    if this.queueCounter > 3 {
        return nil, fmt.Errorf("Sorry queue is full :(")
    }

    videoHash := this.GetVideoHash(videoId, options)

    // Check if this video known to us
    videoStatus, ok := this.GetVideoStatus(videoHash)
    if ok == nil {
        return videoStatus, nil
    }

    ytVideo := new(ytlib.YTVideo).Init(videoId)
    ytVideo.GetFormatList()

    if !ytVideo.HasFormat(options.Format) {
        return nil, fmt.Errorf("Format %d is not available", options.Format)
    }

    // Start Download asynchronously
    go func () {
        this.queueCounter++
        defer this.reduceCounter()

        // Download Video
        downloadDirectory := path.Join(this.downloadDir, videoId, strconv.Itoa(options.Start), strconv.Itoa(options.End))
        os.RemoveAll(downloadDirectory)
        os.MkdirAll(downloadDirectory, 0777)
        filename, _ := ytVideo.DownloadVideo(path.Join(downloadDirectory, videoId), options)
        this.UpdateStatus(videoHash, CONVERTING)

        // Convert Video to frame images
        cmdAvConv := exec.Command("avconv", "-i", filename,
            "-ss", strconv.Itoa(options.Start / 1000),
            "-t", strconv.Itoa((options.End - options.Start) / 1000),
            "-vsync", "1", "-r", "10", "output%05d.gif")
        cmdAvConv.Dir = downloadDirectory
        output, err := cmdAvConv.CombinedOutput()
        if err != nil {
            fmt.Printf("Error converting file: %#v, %v", err.Error(), string(output))
            this.UpdateStatus(videoHash, ERROR)
            return
        }

        // Convert images to animated gif
        cmdConvert := exec.Command("convert", "-delay", "10",
            "-layers", "OptimizeTransparency",
            "output*.gif", videoId + ".gif")
        cmdConvert.Dir = downloadDirectory
        output, err = cmdConvert.CombinedOutput()
        if err != nil {
            fmt.Printf("Error converting file: %#v, %v", err.Error(), string(output))
            this.UpdateStatus(videoHash, ERROR)
            return
        }

        // Remove Temp files
        tmpFiles, _ := filepath.Glob(path.Join(downloadDirectory, "output*.gif"))
        for _, tmpFile := range tmpFiles {
            os.Remove(tmpFile)
        }

        this.UpdateStatus(videoHash, DONE)
    }()

    return this.CreateVideoStatus(videoId, options, DOWNLOADING), nil
}

// Create a video status struct
func (this *DownloadManager) CreateVideoStatus(videoId string, options ytlib.DownloadOptions, status rpctypes.VideoProcessStatus) *rpctypes.VideoStatus {
    videoHash := this.GetVideoHash(videoId, options)
    this.videoList[videoHash] = &rpctypes.VideoStatus { videoHash, status, "" }

    return this.videoList[videoHash]
}

// Get current video status
func (this *DownloadManager) GetVideoStatus(videoHash string) (*rpctypes.VideoStatus, error) {
    videoStatus, exists := this.videoList[videoHash]
    if exists {
        return videoStatus, nil
    }

    return nil, fmt.Errorf("Video status not found")
}

func (this *DownloadManager) UpdateStatus(videoHash string, status rpctypes.VideoProcessStatus) {
    if videoStatus, ok := this.videoList[videoHash]; ok {
        videoStatus.Status = status
        fmt.Println("Updated Status", videoStatus, this.videoList[videoHash])
    }
}

func (this *DownloadManager) reduceCounter () {
    this.queueCounter = this.queueCounter - 1
}

// Create Hash for Video and options
func (this *DownloadManager) GetVideoHash(videoId string, options ytlib.DownloadOptions) string {
    hasher := sha1.New()
    io.WriteString(hasher, fmt.Sprintf("%s-%d-%d-%d", videoId, options.Format, options.Start, options.End))
    return fmt.Sprintf("%x", hasher.Sum(nil))
}
