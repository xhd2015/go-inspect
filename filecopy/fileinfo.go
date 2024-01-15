package filecopy

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"time"

	"github.com/xhd2015/go-vendor-pack/writefs"
)

// FileInfo represents copy source
type FileInfo interface {
	IsFile() bool
	IsDir() bool
	// must return its relative name
	GetPath() string

	// relative to the parent that creates it
	GetName() string
	// Name() string

	// NewerThan tests if this file newer than its target?
	NewerThan(destPath string, destFileInfo os.FileInfo) bool

	Open() (io.Reader, error)
}

type SyncSourcer interface {
	GetSrcFileInfo(srcPath string) (srcFile FileInfo, err error)
	GetSrcChildFiles(srcPath string) (srcFiles []FileInfo, err error)
	GetDestPath(srcPath string) string
}
type rebaseSourcer struct {
	rebaseDir string
}
type mapSourcer struct {
	baseDir    string // can be empty, or "/", are the same
	getContent func(name string) []byte

	sourceNewerChecker func(filePath, destPath string, destFileInfo os.FileInfo) bool
}
type fsSourcer struct {
	baseDir string
	fs      writefs.FS
}

var _ SyncSourcer = ((*rebaseSourcer)(nil))
var _ SyncSourcer = ((*mapSourcer)(nil))
var _ SyncSourcer = (*fsSourcer)(nil)

func (c *rebaseSourcer) GetSrcFileInfo(srcFilePath string) (srcFile FileInfo, err error) {
	return fsGetSrcFileInfo(writefs.SysFS{}, srcFilePath)

}
func (c *rebaseSourcer) GetSrcChildFiles(srcDir string) (srcFiles []FileInfo, err error) {
	return fsGetSrcChildFiles(writefs.SysFS{}, srcDir)
}

func (c *rebaseSourcer) GetDestPath(srcPath string) string {
	return path.Join(c.rebaseDir, srcPath)
}

// GetDestPath implements SyncSourcer.
func (c *fsSourcer) GetDestPath(srcPath string) string {
	return path.Join(c.baseDir, srcPath)
}

func (c *fsSourcer) GetSrcFileInfo(srcPath string) (srcFile FileInfo, err error) {
	return fsGetSrcFileInfo(c.fs, srcPath)
}
func (c *fsSourcer) GetSrcChildFiles(srcDir string) (srcFiles []FileInfo, err error) {
	return fsGetSrcChildFiles(c.fs, srcDir)
}

func fsGetSrcFileInfo(fs writefs.FS, srcPath string) (srcFile FileInfo, err error) {
	osFile, err := fs.Stat(srcPath)
	if err != nil {
		return
	}
	srcFile = NewFileInfo(fs, srcPath, osFile)
	return
}

func fsGetSrcChildFiles(fs writefs.FS, srcDir string) (srcFiles []FileInfo, err error) {
	osFiles, err := fs.ReadDir(srcDir)
	if err != nil {
		return
	}
	srcFiles = NewFileInfos(fs, srcDir, osFiles)
	return
}

func (c *mapSourcer) GetSrcFileInfo(srcFilePath string) (srcFile FileInfo, err error) {
	return &mapFile{
		path:               path.Join(c.baseDir, srcFilePath),
		relativePath:       srcFilePath,
		content:            c.getContent(srcFilePath),
		sourceNewerChecker: c.sourceNewerChecker,
	}, nil
}
func (c *mapSourcer) GetSrcChildFiles(srcDir string) (srcFiles []FileInfo, err error) {
	panic(fmt.Errorf("should not call GetSrcChildFiles of mapSourcer"))
}

func (c *mapSourcer) GetDestPath(srcPath string) string {
	return path.Join(c.baseDir, srcPath)
}
func GetEarliestModTime(filePath string) (modTime time.Time, err error) {
	srcFile, err := os.Stat(filePath)
	if err != nil {
		return
	}
	if srcFile.IsDir() {
		var files []os.FileInfo
		files, err = ioutil.ReadDir(filePath)
		if err != nil {
			return
		}
		if len(files) == 0 {
			return
		}
		minModTime := files[0].ModTime()
		for _, f := range files[1:] {
			fMod := f.ModTime()
			if fMod.Before(minModTime) {
				minModTime = fMod
			}
		}
		modTime = minModTime
	} else {
		modTime = srcFile.ModTime()
	}
	return
}

func GetNewestModTime(filePath string) (modTime time.Time, err error) {
	srcFile, err := os.Stat(filePath)
	if err != nil {
		return
	}
	if srcFile.IsDir() {
		var files []os.FileInfo
		files, err = ioutil.ReadDir(filePath)
		if err != nil {
			return
		}
		if len(files) == 0 {
			return
		}
		maxModTime := files[0].ModTime()
		for _, f := range files[1:] {
			fMod := f.ModTime()
			if fMod.After(maxModTime) {
				maxModTime = fMod
			}
		}
		modTime = maxModTime
	} else {
		modTime = srcFile.ModTime()
	}
	return
}

//  osFiles
type osFileInfo struct {
	path string
	f    os.FileInfo
}

func NewFileInfo(fs writefs.FS, path string, f os.FileInfo) FileInfo {
	if _, ok := fs.(writefs.SysFS); ok {
		return &osFileInfo{path: path, f: f}
	}
	return &fsFileInfo{fs: fs, osFileInfo: osFileInfo{path: path, f: f}}
}
func NewFileInfos(fs writefs.FS, dir string, f []os.FileInfo) []FileInfo {
	fileInfos := make([]FileInfo, 0, len(f))
	for _, x := range f {
		fileInfos = append(fileInfos, NewFileInfo(fs, path.Join(dir, x.Name()), x))
	}
	return fileInfos
}

func (c *osFileInfo) IsFile() bool {
	return c.f.Mode().IsRegular()
}
func (c *osFileInfo) IsDir() bool {
	return c.f.IsDir()
}
func (c *osFileInfo) GetPath() string {
	return c.path
}
func (c *osFileInfo) GetName() string {
	return c.f.Name()
}
func (c *osFileInfo) NewerThan(destPath string, destFileInfo os.FileInfo) bool {
	// must only copy regular file
	if !destFileInfo.Mode().IsRegular() {
		return true
	}
	// compare mod time
	return c.f.ModTime().After(destFileInfo.ModTime())
}
func (c *osFileInfo) Open() (io.Reader, error) {
	return os.Open(c.path)
}

type fsFileInfo struct {
	fs writefs.FS
	osFileInfo
}

func (c *fsFileInfo) Open() (io.Reader, error) {
	return c.fs.OpenFileRead(c.path)
}

// mapFile
type mapFile struct {
	path         string
	relativePath string
	content      []byte

	// cached
	// statInfo os.FileInfo

	sourceNewerChecker func(filePath, destPath string, destFileInfo os.FileInfo) bool
}

var _ FileInfo = ((*mapFile)(nil))

func (c *mapFile) IsFile() bool {
	return true
}
func (c *mapFile) IsDir() bool {
	return false
}
func (c *mapFile) GetPath() string {
	return c.path
}
func (c *mapFile) GetName() string {
	return c.relativePath
}
func (c *mapFile) NewerThan(destPath string, destFileInfo os.FileInfo) bool {
	return c.sourceNewerChecker(c.path, destPath, destFileInfo)
}

// func (c *mapFile) getStatInfo() os.FileInfo {
// 	if c.statInfo != nil {
// 		statInfo, err := os.Stat(c.path)
// 		if err != nil {
// 			panic(err)
// 		}
// 		c.statInfo = statInfo
// 	}
// 	return c.statInfo
// }

func (c *mapFile) Open() (io.Reader, error) {
	return bytes.NewReader(c.content), nil
}
