package op

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/utils/log"
)

// relayLogImageBudgetBytes 是 log-images 目录内本地图片二进制的固定总预算。
// 预算覆盖仍占用磁盘的全部缓存文件，含正在写入的保留量与删除失败的待重试项；超出后不再缓存新预览。
const relayLogImageBudgetBytes = 512 << 20

const (
	relayLogImageNamePrefix = "image-"
	relayLogImageDirMode    = os.FileMode(0o700)
	relayLogImageFileMode   = os.FileMode(0o600)
)

// errRelayLogImageBudget 表示继续写入会超出本地图片总预算，必须放弃该预览。
var errRelayLogImageBudget = errors.New("relay log image budget exceeded")

// relayLogStoredImage 是 store 内部单张图片的私有元信息。
// 本地缓存只保留文件路径、MIME 与二进制大小，远程图片只保留原始 http(s) 地址；两者都不保留输入 Base64。
type relayLogStoredImage struct {
	file string
	url  string
	mime string
	size int64
}

func (image relayLogStoredImage) local() bool {
	return image.file != ""
}

// relayLogImageFiles 管理 log-images 目录、二进制总预算与本地文件的读租约：
// budget 为已占用字节（含正在写入的保留量），refs 为打开中的读租约计数，
// retired 记录已停止引用但仍有租约、待最后一个 Close 后删除的文件，
// pending 记录删除失败、留待下一有意义缓存操作重试的文件。
// 未成功初始化时 enabled 为 false：不访问默认目录、不落盘，只保留远程 URL。
type relayLogImageFiles struct {
	mu      sync.Mutex
	dir     string
	enabled bool
	seq     uint64
	budget  int64
	refs    map[string]int
	retired map[string]int64
	pending map[string]int64
}

var relayLogImages = relayLogImageFiles{
	refs:    make(map[string]int),
	retired: make(map[string]int64),
	pending: make(map[string]int64),
}

// InitRelayLogImageFiles 准备日志图片缓存目录：创建目录（0700）并清掉上次遗留的 image-* 普通文件。
// 失败时返回错误，调用方应记录 warn 并继续服务；初始化未成功时本地预览被禁用，远程 URL 仍保留。
func InitRelayLogImageFiles(directory string) error {
	if strings.TrimSpace(directory) == "" {
		return errors.New("relay log image directory is empty")
	}
	if err := os.MkdirAll(directory, relayLogImageDirMode); err != nil {
		return fmt.Errorf("create relay log image directory: %w", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read relay log image directory: %w", err)
	}
	cleaned := 0
	remaining := make(map[string]int64)
	var budget int64
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasPrefix(entry.Name(), relayLogImageNamePrefix) {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			// 删不掉的遗留文件继续占用预算，记 warn 等待后续缓存操作重试。
			log.Warnf("relay log image startup cleanup failed: %v", err)
			var size int64
			if info, statErr := entry.Info(); statErr == nil {
				size = info.Size()
			}
			remaining[path] = size
			budget += size
			continue
		}
		cleaned++
	}
	relayLogImages.mu.Lock()
	relayLogImages.dir = directory
	relayLogImages.enabled = true
	relayLogImages.seq = 0
	relayLogImages.budget = budget
	relayLogImages.refs = make(map[string]int)
	relayLogImages.retired = make(map[string]int64)
	relayLogImages.pending = remaining
	relayLogImages.mu.Unlock()
	log.Debugf("relay log image cache ready: dir=%s cleaned=%d", directory, cleaned)
	return nil
}

// prepareRelayLogImages 在 store 锁外把交付图片转换为 store 私有元信息：
// 内联 data URL 流式解码落盘，远程 http(s) 地址原样保留。
func prepareRelayLogImages(id int64, images []model.RelayLogImage) []relayLogStoredImage {
	if len(images) == 0 {
		return nil
	}
	stored := make([]relayLogStoredImage, 0, len(images))
	for _, image := range images {
		if entry, ok := prepareRelayLogImage(id, image.Src); ok {
			stored = append(stored, entry)
		}
	}
	if len(stored) == 0 {
		return nil
	}
	return stored
}

func prepareRelayLogImage(id int64, src string) (relayLogStoredImage, bool) {
	if src == "" {
		return relayLogStoredImage{}, false
	}
	if isRelayLogImageURL(src) {
		return relayLogStoredImage{url: src}, true
	}
	header, encoded, found := strings.Cut(src, ",")
	if !found {
		return relayLogStoredImage{}, false
	}
	// 保存静态 MIME 常量，不能保存 src 子串，否则会保住整张图片的 Base64 底层字符串。
	var mime string
	switch header {
	case "data:image/png;base64":
		mime = "image/png"
	case "data:image/jpeg;base64":
		mime = "image/jpeg"
	case "data:image/webp;base64":
		mime = "image/webp"
	default:
		return relayLogStoredImage{}, false
	}
	path, size, err := relayLogImages.writeImage(encoded)
	if err != nil {
		if errors.Is(err, errRelayLogImageCacheDisabled) {
			log.Debugf("relay log %d image preview skipped: %v", id, err)
			return relayLogStoredImage{}, false
		}
		log.Warnf("relay log %d image not cached: %v", id, err)
		return relayLogStoredImage{}, false
	}
	return relayLogStoredImage{file: path, mime: mime, size: size}, true
}

// isRelayLogImageURL 判断图片源是否为可交给浏览器直接加载的远程地址。
func isRelayLogImageURL(src string) bool {
	parsed, err := url.Parse(src)
	return err == nil && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

var errRelayLogImageCacheDisabled = errors.New("relay log image cache is disabled")

// writeImage 流式解码 Base64 并写入一个新缓存文件；中途失败或超额时删除半成品并回收预留预算。
func (files *relayLogImageFiles) writeImage(encoded string) (string, int64, error) {
	files.retryPending()
	files.mu.Lock()
	if !files.enabled {
		files.mu.Unlock()
		return "", 0, errRelayLogImageCacheDisabled
	}
	if files.budget >= relayLogImageBudgetBytes {
		files.mu.Unlock()
		return "", 0, errRelayLogImageBudget
	}
	files.seq++
	name := fmt.Sprintf("%s%d-%d", relayLogImageNamePrefix, os.Getpid(), files.seq)
	path := filepath.Join(files.dir, name)
	files.mu.Unlock()

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, relayLogImageFileMode)
	if err != nil {
		return "", 0, fmt.Errorf("create relay log image file: %w", err)
	}
	writer := &relayLogImageWriter{file: file, files: files, path: path}
	_, copyErr := io.Copy(writer, base64.NewDecoder(base64.StdEncoding, strings.NewReader(encoded)))
	closeErr := file.Close()
	if copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		writer.abort()
		return "", 0, copyErr
	}
	return path, writer.written, nil
}

// relayLogImageWriter 在写入过程中按字节预占预算，超限立即失败；不做整图解码，也不一次性分配整图内存。
type relayLogImageWriter struct {
	file    *os.File
	files   *relayLogImageFiles
	path    string
	written int64
}

func (writer *relayLogImageWriter) Write(data []byte) (int, error) {
	if !writer.files.reserveBudget(int64(len(data))) {
		return 0, errRelayLogImageBudget
	}
	written, err := writer.file.Write(data)
	if written < len(data) {
		writer.files.releaseBudget(int64(len(data) - written))
	}
	writer.written += int64(written)
	return written, err
}

// abort 丢弃半成品：文件删除成功才回收预算，删除失败则保留预算交给后续重试与启动清理。
func (writer *relayLogImageWriter) abort() {
	writer.files.discard(writer.path, writer.written)
}

func (files *relayLogImageFiles) reserveBudget(size int64) bool {
	files.mu.Lock()
	defer files.mu.Unlock()
	if !files.enabled || files.budget+size > relayLogImageBudgetBytes {
		return false
	}
	files.budget += size
	return true
}

func (files *relayLogImageFiles) releaseBudget(size int64) {
	if size <= 0 {
		return
	}
	files.mu.Lock()
	files.budget -= size
	if files.budget < 0 {
		files.budget = 0
	}
	files.mu.Unlock()
}

// acquire 为一次读取增加租约；缓存禁用或文件已进入待删除状态时拒绝。
func (files *relayLogImageFiles) acquire(path string) error {
	files.mu.Lock()
	defer files.mu.Unlock()
	if !files.enabled {
		return errRelayLogImageCacheDisabled
	}
	if _, retired := files.retired[path]; retired {
		return os.ErrNotExist
	}
	files.refs[path]++
	return nil
}

// releaseLease 结束一次读取；文件已停止引用且再无租约时，此刻才删除（Windows 打开中的文件无法删除）。
func (files *relayLogImageFiles) releaseLease(path string) {
	files.mu.Lock()
	if files.refs[path] > 1 {
		files.refs[path]--
		files.mu.Unlock()
		return
	}
	delete(files.refs, path)
	size, retired := files.retired[path]
	if !retired {
		files.mu.Unlock()
		return
	}
	delete(files.retired, path)
	files.mu.Unlock()
	files.discard(path, size)
}

// retire 停止引用一张本地图片：无打开的读租约时立即删除，否则等最后一个 Close 后再删。
func (files *relayLogImageFiles) retire(path string, size int64) {
	if path == "" {
		return
	}
	files.mu.Lock()
	if files.refs[path] > 0 {
		files.retired[path] = size
		files.mu.Unlock()
		return
	}
	files.mu.Unlock()
	files.discard(path, size)
}

// discard 删除一张不再被引用的本地文件并回收预算；删除失败时保留预算，记 warn 留待后续重试。
func (files *relayLogImageFiles) discard(path string, size int64) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		log.Warnf("relay log image file remove failed: %v", err)
		files.mu.Lock()
		files.pending[path] = size
		files.mu.Unlock()
		return
	}
	files.mu.Lock()
	files.budget -= size
	if files.budget < 0 {
		files.budget = 0
	}
	delete(files.pending, path)
	files.mu.Unlock()
}

// retryPending 在下一有意义缓存操作（写入新预览或启动初始化）时重试此前删除失败的缓存文件。
func (files *relayLogImageFiles) retryPending() {
	files.mu.Lock()
	if len(files.pending) == 0 {
		files.mu.Unlock()
		return
	}
	pending := files.pending
	files.pending = make(map[string]int64)
	files.mu.Unlock()
	for path, size := range pending {
		files.discard(path, size)
	}
}

// relayLogImageLease 是本地图片的读租约：持有时文件不会被删除，Close 后若文件已停止引用则立即删除。
type relayLogImageLease struct {
	file *os.File
	path string
	once sync.Once
}

func (lease *relayLogImageLease) Read(data []byte) (int, error) {
	return lease.file.Read(data)
}

func (lease *relayLogImageLease) Seek(offset int64, whence int) (int64, error) {
	return lease.file.Seek(offset, whence)
}

func (lease *relayLogImageLease) Close() error {
	err := lease.file.Close()
	lease.once.Do(func() { relayLogImages.releaseLease(lease.path) })
	return err
}

// relayLogImageSource 返回客户端可直接渲染的图片地址：本地缓存使用同组鉴权路由，远程地址原样返回。
func relayLogImageSource(id int64, index int, image relayLogStoredImage) string {
	if image.local() {
		return fmt.Sprintf("/api/v1/log/%d/images/%d", id, index)
	}
	return image.url
}

func relayLogImagesFromStored(id int64, stored []relayLogStoredImage) []model.RelayLogImage {
	if len(stored) == 0 {
		return nil
	}
	images := make([]model.RelayLogImage, 0, len(stored))
	for index, image := range stored {
		images = append(images, model.RelayLogImage{Src: relayLogImageSource(id, index, image)})
	}
	return images
}

// retireRelayLogStoredImages 释放一组图片文件（立即删除，或等最后一个读租约结束）。
func retireRelayLogStoredImages(images []relayLogStoredImage) {
	for _, image := range images {
		relayLogImages.retire(image.file, image.size)
	}
}

// RelayLogStoreOpenImage 打开一条日志记录中第 index 张本地缓存图片。
// 记录或图片不存在时返回可用 errors.Is 匹配 os.ErrNotExist 的错误；远程 URL 图片不由服务端提供。
func RelayLogStoreOpenImage(id int64, index int) (io.ReadSeekCloser, string, error) {
	relayLogStore.RLock()
	record, ok := relayLogStore.records[id]
	if !ok {
		relayLogStore.RUnlock()
		return nil, "", os.ErrNotExist
	}
	if index < 0 || index >= len(record.responseImages) {
		relayLogStore.RUnlock()
		return nil, "", os.ErrNotExist
	}
	image := record.responseImages[index]
	if !image.local() {
		relayLogStore.RUnlock()
		return nil, "", os.ErrNotExist
	}
	// 先在记录仍受保护时取得租约，再释放 store 锁；清理不能抢先删除即将读取的文件。
	err := relayLogImages.acquire(image.file)
	relayLogStore.RUnlock()
	if err != nil {
		return nil, "", os.ErrNotExist
	}
	file, err := os.Open(image.file)
	if err != nil {
		relayLogImages.releaseLease(image.file)
		if os.IsNotExist(err) {
			return nil, "", os.ErrNotExist
		}
		return nil, "", fmt.Errorf("open relay log image: %w", err)
	}
	return &relayLogImageLease{file: file, path: image.file}, image.mime, nil
}
