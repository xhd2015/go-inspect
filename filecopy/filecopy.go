package filecopy

// TODO: make channel optional

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xhd2015/go-vendor-pack/writefs"
)

// sync model:
//     files -> rebaseDir/${file}
// file can either be file or dir, but links are ignored.

type SyncRebaseOptions struct {
	// Ignores tells the syncer that these dirs are not synced
	Ignores []string

	// Delete missing files
	DeleteNotFound bool

	Force bool

	OnUpdateStats func(total int64, finished int64, copied int64, lastStat bool)

	ProcessDestPath func(s string) string

	DidCopy func(srcPath string, destPath string)

	ShouldCopyFile func(srcPath string, destPath string, srcFileInfo FileInfo, destFile fs.FileInfo) (bool, error)

	// target filesystem
	FS writefs.FS
}

func SyncRebase(initPaths []string, rebaseDir string, opts SyncRebaseOptions) error {
	return Sync(initPaths, &rebaseSourcer{rebaseDir: rebaseDir}, opts)
}
func SyncFS(srcFS writefs.FS, initPaths []string, targetBaseDir string, opts SyncRebaseOptions) error {
	return doSync(func(fn func(path string)) {
		for _, p := range initPaths {
			fn(p)
		}
	}, &fsSourcer{
		baseDir: targetBaseDir,
		fs:      srcFS,
	}, opts)
}

// SyncGeneratedMap
// when `sourceNewerChecker` returns true, the target file is overwritten.
func SyncGeneratedMap(contents map[string][]byte, targetBaseDir string, sourceNewerChecker func(filePath, destPath string, destFileInfo os.FileInfo) bool, opts SyncRebaseOptions) error {
	return doSync(func(fn func(path string)) {
		for path := range contents {
			fn(path)
		}
	}, &mapSourcer{
		baseDir: targetBaseDir,
		getContent: func(name string) []byte {
			content, ok := contents[name]
			if !ok {
				panic(fmt.Errorf("unexpected name:%v", name))
			}
			return content
		},
		sourceNewerChecker: sourceNewerChecker,
	}, opts)
}
func SyncGenerated(ranger func(fn func(path string)), contentGetter func(name string) []byte, targetBaseDir string, sourceNewerChecker func(filePath, destPath string, destFileInfo os.FileInfo) bool, opts SyncRebaseOptions) error {
	return doSync(ranger, &mapSourcer{
		baseDir:            targetBaseDir,
		getContent:         contentGetter,
		sourceNewerChecker: sourceNewerChecker,
	}, opts)
}

// SyncRebaseContents implements wise file sync, srcs are all sync into `rebaseDir`
// for generated contents, there is no physical content,
func Sync(initPaths []string, sourcer SyncSourcer, opts SyncRebaseOptions) error {
	return doSync(func(fn func(path string)) {
		for _, path := range initPaths {
			fn(path)
		}
	}, sourcer, opts)
}

// primary bottleneck: read FS, can be made
// async
func doSync(ranger func(fn func(path string)), sourcer SyncSourcer, opts SyncRebaseOptions) error {
	var fsHandle writefs.FS = writefs.SysFS{}
	// var fsHandle FS = NoopFS{}
	// var fsHandle FS = &mapFS

	if opts.FS != nil {
		fsHandle = opts.FS
	}

	shouldCopyFile := opts.ShouldCopyFile
	shouldIgnore := newRegexMatcher(opts.Ignores)
	readDestDir := func(dest string) (destFiles []os.FileInfo, destDirMade bool, err error) {
		destFiles, readErr := fsHandle.ReadDir(dest)
		if readErr == nil {
			destDirMade = true
			return
		}
		if writefs.IsNotExist(readErr) {
			return
		}
		err = readErr
		// rare case: is a file
		fs, fsErr := fsHandle.Stat(dest)
		if fsErr == nil && !fs.IsDir() {
			// TODO: add an option to indicate overwrite
			rmErr := fsHandle.RemoveFile(dest)
			if rmErr != nil {
				err = fmt.Errorf("remove existing dest file error:%w", rmErr)
				return
			}
			err = nil
		}
		// may have err
		return
	}
	var totalFiles int64
	var finishedFiles int64
	var copiedFiles int64

	lastStat := false

	var lastWarnMassiveFiles time.Time
	onUpdateStats := func() {
		totalFiles := atomic.LoadInt64(&totalFiles)
		// warn every 30s for massive files copy
		if totalFiles >= 300000 && (lastWarnMassiveFiles.IsZero() || time.Since(lastWarnMassiveFiles) >= 30*time.Second) {
			finishedFileNum := atomic.LoadInt64(&finishedFiles)
			lastWarnMassiveFiles = time.Now()
			log.Printf("WARNING: decreased performance due to massive files: %d / %d", finishedFileNum, totalFiles)
		}
		if opts.OnUpdateStats != nil {
			opts.OnUpdateStats(totalFiles, atomic.LoadInt64(&finishedFiles), atomic.LoadInt64(&copiedFiles), lastStat)
		}
	}
	didCopy := func(srcPath, destPath string) {
		if opts.DidCopy != nil {
			opts.DidCopy(srcPath, destPath)
		}
	}
	processDestPath := func(s string) string {
		if opts.ProcessDestPath == nil {
			return s
		}
		return opts.ProcessDestPath(s)
	}

	// wait of goroutines
	var wg sync.WaitGroup

	const chSize = 1000
	var gNum = 50 // 200M memory at most
	goNumStr := os.Getenv("GO_INSPECT_FILE_COPY_GO_NUM")
	if goNumStr != "" {
		v, _ := strconv.ParseInt(goNumStr, 10, 64)
		if v > 0 {
			log.Printf("file copy go num: %d", v)
			gNum = int(v)
		}
	}

	type copyInfo struct {
		path         string
		destPath     string
		fileInfo     FileInfo
		destFileInfo fs.FileInfo
	}

	filesCh := make(chan copyInfo, chSize)

	var hasErr int32 // 0:false 1:true

	var mutext sync.Mutex
	var panicErr interface{}

	// generated file will always go handleFile, no handleDir called
	handleFile := func(buf []byte, srcPath string, srcFileInfo FileInfo, destPath string, destFile fs.FileInfo) error {
		var err error
		if destFile != nil && !destFile.Mode().IsRegular() {
			// delete dest file if not a regular file,becuase we are about to truncate it
			// isDir && !isRegular can be true at the same time.
			err = fsHandle.RemoveAll(destPath)
			if err != nil {
				return err
			}
		}

		// copy file
		didCopy(srcPath, destPath)
		err = copyFile(fsHandle, srcFileInfo, destPath, buf)
		if err != nil {
			return err
		}

		atomic.AddInt64(&copiedFiles, 1)
		atomic.AddInt64(&finishedFiles, 1)
		onUpdateStats()
		return nil
	}
	// handleDirOrFile process file paths,
	// if the path is a directory, it walks on it
	// otherwise it sends the file to channel for copy
	var handleDirOrFile func(filePath string, fileInfo FileInfo, destFileInfo fs.FileInfo, destFileInfoResolved bool) error

	handleDirOrFile = func(filePath string, fileInfo FileInfo, destFileInfo fs.FileInfo, destFileInfoResolved bool) error {
		if shouldIgnore(filePath) {
			// fmt.Printf("DEBUG ignore file:%v\n", srcPath)
			return nil
		}

		destPath := processDestPath(sourcer.GetDestPath(filePath))

		var err error
		if fileInfo == nil {
			fileInfo, err = sourcer.GetSrcFileInfo(filePath)
			if err != nil {
				return err
			}
		}
		if !fileInfo.IsDir() {
			if !fileInfo.IsFile() {
				// not a dir nor a file,so nothing to do
				return nil
			}
			atomic.AddInt64(&totalFiles, 1)
			onUpdateStats()

			// check if we should copy the file
			shouldCopy := true
			if !opts.Force {
				if !destFileInfoResolved {
					var statErr error
					destFileInfo, statErr = fsHandle.Stat(destPath)
					if statErr != nil && !writefs.IsNotExist(statErr) {
						return statErr
					}
				}
				if destFileInfo != nil {
					if shouldCopyFile != nil {
						shouldCopy, err = shouldCopyFile(filePath, destPath, fileInfo, destFileInfo)
						if err != nil {
							return err
						}
					} else {
						shouldCopy = fileInfo.NewerThan(destPath, destFileInfo)
					}
				}
			}

			if !shouldCopy {
				atomic.AddInt64(&finishedFiles, 1)
				onUpdateStats()
				return nil
			}
			// write to filesChannel to consume
			filesCh <- copyInfo{path: filePath, destPath: destPath, fileInfo: fileInfo, destFileInfo: destFileInfo}

			return nil
		}
		// handle dirs
		destFiles, destDirMade, err := readDestDir(destPath)
		if err != nil {
			return err
		}

		// create target dirs
		if !destDirMade {
			err = fsHandle.MkdirAll(destPath, 0755)
			if err != nil {
				return fmt.Errorf("create dest dir error:%v", err)
			}
			destDirMade = true
		}

		childSrcFiles, err := sourcer.GetSrcChildFiles(filePath)
		if err != nil {
			return fmt.Errorf("read src dir error:%v", err)
		}

		var destMap map[string]os.FileInfo
		var missingInSrc map[string]bool
		if len(destFiles) > 0 {
			destMap = make(map[string]os.FileInfo, len(destFiles))
			missingInSrc = make(map[string]bool, len(destFiles))
			for _, destFile := range destFiles {
				// NOTE: very prone to bug: if directly take address of destFile, you will get wrong result
				// d := destFile
				// destMap[destFile.Name()] = &d

				destMap[destFile.Name()] = destFile
				missingInSrc[destFile.Name()] = true
			}
		}
		for _, childSrcFile := range childSrcFiles {
			fileName := childSrcFile.GetName()
			// mark no delete
			if _, ok := missingInSrc[fileName]; ok {
				missingInSrc[fileName] = false
			}
			err = handleDirOrFile(childSrcFile.GetPath(), childSrcFile, destMap[fileName], true)
			if err != nil {
				return err
			}
		}

		// TODO may handle in a separate goroutine
		if opts.DeleteNotFound {
			// remove missing names
			for name, missing := range missingInSrc {
				if missing {
					err = fsHandle.RemoveAll(path.Join(destPath, name))
					if err != nil {
						return fmt.Errorf("remove file error:%v", err)
					}
				}
			}
		}

		return nil
	}

	var res sync.Map
	exhaustFilesCh := func() {
		defer func() {
			if e := recover(); e != nil {
				atomic.StoreInt32(&hasErr, 1)
				if panicErr == nil {
					mutext.Lock()
					if panicErr == nil {
						panicErr = e
					}
					mutext.Unlock()
				}
			}
		}()
		var buf []byte
		for copyInfo := range filesCh {
			if atomic.LoadInt32(&hasErr) == 1 {
				return
			}
			if buf == nil {
				buf = make([]byte, 0, 4*1024*1024) // 4MB
			}
			err := handleFile(buf, copyInfo.path, copyInfo.fileInfo, copyInfo.destPath, copyInfo.destFileInfo)
			if err != nil {
				atomic.StoreInt32(&hasErr, 1)
				res.Store(copyInfo.path, err)
				return
			}
		}
	}

	// start goroutines to copy files
	for i := 0; i < gNum; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			exhaustFilesCh()
		}()
	}

	var walkErr error
	// walk initial roots
	ranger(func(path string) {
		if walkErr != nil {
			return
		}
		walkErr = handleDirOrFile(path, nil, nil, false)
	})

	close(filesCh)
	wg.Wait()

	// fmt.Printf("DEBUG: total files:%d, copied: %d\n", totalFiles, copiedFiles)
	lastStat = true
	onUpdateStats()

	if walkErr != nil {
		return fmt.Errorf("walk dir: %w", walkErr)
	}
	if panicErr != nil {
		return fmt.Errorf("panic: %v", panicErr)
	}

	var errList []string
	res.Range(func(key, value interface{}) bool {
		errList = append(errList, fmt.Sprintf("dir:%v %v", key, value))
		return true
	})

	if len(errList) > 0 {
		return fmt.Errorf("%s", strings.Join(errList, ";"))
	}

	return nil
}

func newRegexMatcher(re []string) func(s string) bool {
	if len(re) == 0 {
		return func(s string) bool {
			return false
		}
	}
	regex := make([]*regexp.Regexp, len(re))
	return func(s string) bool {
		for i, r := range re {
			rg := regex[i]
			if rg == nil {
				rg = regexp.MustCompile(r)
				regex[i] = rg
			}
			if rg.MatchString(s) {
				return true
			}
		}
		return false
	}
}

func copyFile(fs writefs.FS, srcFile FileInfo, destPath string, buf []byte) (err error) {
	// defer func() {
	// 	fmt.Printf("DEBUG copy file DONE:%v\n", srcFile)
	// }()
	// fmt.Printf("DEBUG copy file:%v\n", srcFile)
	// if srcFile != "/Users/x/gopath/src/xxx.log" {
	// 	return nil
	// }
	// fmt.Printf("DEBUG copy %v/%v\n", srcDir, name)
	srcFileIO, err := srcFile.Open()
	// srcFileIO, err := os.OpenFile(srcPath, os.O_RDONLY, 0777)
	if err != nil {
		err = fmt.Errorf("open src file error: %w", err)
		return
	}
	if srcFileIOCloser, ok := srcFileIO.(io.Closer); ok {
		defer srcFileIOCloser.Close()
	}
	// TODO: add a flag to indicate whether mkdir is needed
	err = fs.MkdirAll(path.Dir(destPath), 0777)
	if err != nil {
		err = fmt.Errorf("create dir %v error:%v", path.Dir(destPath), err)
		return
	}
	destFileIO, err := fs.OpenFileWrite(destPath)
	if err != nil {
		err = fmt.Errorf("create dest file error:%v", err)
		return
	}
	defer func() {
		destFileIO.Close()
		if err == nil {
			if os, ok := fs.(writefs.FSWithTime); ok {
				// fsrc, _ := os.Stat(srcFile)
				// fb, _ := os.Stat(destName)
				// fmt.Printf("DEBUG before modify time:%v -> %v\n", destName, fb.ModTime())
				// now := fsrc.ModTime().Add(24 * time.Hour)
				// fmt.Printf("DEBUG will modify time:%v -> %v\n", destName, now)
				now := time.Now()
				err = os.Chtimes(destPath, now, now)
				// f, _ := os.Stat(destName)
				// fmt.Printf("DEBUG after modify time:%v -> %v\n", destName, f.ModTime())
			}

		}
	}()

	for {
		var n int
		n, err = srcFileIO.Read(buf[0:cap(buf)])
		if n > 0 {
			var m int
			m, err = destFileIO.Write(buf[:n])
			if err != nil {
				err = fmt.Errorf("write dest file error:%v", err)
				return
			}
			if m != n {
				err = fmt.Errorf("copy file error, unexpected written bytes:%d, want:%d", m, n)
				return
			}
		}
		if errors.Is(err, io.EOF) {
			err = nil
			return
		}
		if err != nil {
			return
		}
	}
}

func NewLogger(log func(format string, args ...interface{}), enable bool, enableLast bool, interval time.Duration) func(total, finished, copied int64, lastStat bool) {
	var last time.Time
	return func(total, finished, copied int64, lastStat bool) {
		if !enable {
			if !lastStat || !enableLast {
				return
			}
		}
		now := time.Now()
		if !lastStat && now.Sub(last) < interval {
			return
		}
		last = now
		var percent float64
		if total > 0 {
			percent = float64(finished) / float64(total) * 100
		}
		log("copy %.2f%% total:%4d, finished:%4d, changed:%4d", percent, total, finished, copied)
		if lastStat {
			log("copy finished")
		}
	}
}
