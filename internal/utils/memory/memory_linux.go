//go:build linux

package memory

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
)

// Read 每次读取宿主内存和当前进程可见的 cgroup 限额，不缓存动态值。
// cgroup 信息不完整时 Available 为零并返回错误，不能据此使用宿主余量。
func Read() (Status, error) {
	status, err := readMeminfo()
	if err != nil {
		return Status{}, err
	}
	if err := restrictCgroup(&status); err != nil {
		status.Available = 0
		return status, err
	}
	return status, nil
}

func readMeminfo() (Status, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return Status{}, fmt.Errorf("read memory status: %w", err)
	}
	var status Status
	var haveTotal, haveAvailable bool
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "MemTotal:" && fields[0] != "MemAvailable:" {
			continue
		}
		if len(fields) != 3 || fields[2] != "kB" {
			return Status{}, fmt.Errorf("/proc/meminfo: invalid %s", fields[0])
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return Status{}, fmt.Errorf("/proc/meminfo: parse %s: %w", fields[0], err)
		}
		if value > ^uint64(0)/1024 {
			return Status{}, fmt.Errorf("/proc/meminfo: %s overflows bytes", fields[0])
		}
		if fields[0] == "MemTotal:" {
			status.Total, haveTotal = value*1024, true
		} else {
			status.Available, haveAvailable = value*1024, true
		}
	}
	if !haveTotal || !haveAvailable || status.Total == 0 {
		return Status{}, fmt.Errorf("/proc/meminfo: missing MemTotal/MemAvailable or zero MemTotal")
	}
	status.Available = min(status.Available, status.Total)
	return status, nil
}

type cgroupMount struct {
	root  string
	point string
}

func restrictCgroup(status *Status) error {
	data, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return fmt.Errorf("read memory cgroup: %w", err)
	}
	group, v2, err := memoryCgroup(string(data))
	if err != nil || group == "" {
		return err
	}
	data, err = os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return fmt.Errorf("read memory cgroup mounts: %w", err)
	}
	mounts, err := memoryMounts(string(data), v2)
	if err != nil {
		return err
	}
	// 优先按 mount root 映射；cgroup 命名空间可能将成员路径改为相对其根的路径。
	for pass := 0; pass < 2; pass++ {
		matched := false
		for _, mount := range mounts {
			direct := group == mount.root || mount.root == "/" || strings.HasPrefix(group, mount.root+"/")
			if direct != (pass == 0) {
				continue
			}
			relative := group
			if direct {
				relative = strings.TrimPrefix(group, mount.root)
			}
			dir := path.Join(mount.point, strings.TrimPrefix(relative, "/"))
			info, err := os.Stat(dir)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return fmt.Errorf("resolve memory cgroup: %w", err)
			}
			if !info.IsDir() {
				return fmt.Errorf("memory cgroup %s is not a directory", dir)
			}
			matched = true
			if err := restrictHierarchy(status, dir, mount, v2); err != nil {
				return err
			}
		}
		if matched {
			return nil
		}
	}
	return fmt.Errorf("no visible memory cgroup mount for %s", group)
}

func memoryCgroup(data string) (string, bool, error) {
	var unified string
	for _, line := range strings.Split(data, "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, ":", 3)
		if len(fields) != 3 {
			return "", false, fmt.Errorf("/proc/self/cgroup: malformed entry")
		}
		v2 := fields[0] == "0" && fields[1] == ""
		if !v2 && !hasController(fields[1], "memory") {
			continue
		}
		group, err := cgroupPath(fields[2])
		if err != nil {
			return "", false, fmt.Errorf("/proc/self/cgroup: %w", err)
		}
		if !v2 {
			// memory 控制器不能同时归属 v1 和 v2。
			return group, false, nil
		}
		unified = group
	}
	return unified, true, nil
}

func memoryMounts(data string, v2 bool) ([]cgroupMount, error) {
	var mounts []cgroupMount
	for _, line := range strings.Split(data, "\n") {
		if line == "" {
			continue
		}
		left, right, ok := strings.Cut(line, " - ")
		fields, filesystem := strings.Fields(left), strings.Fields(right)
		if !ok || len(fields) < 6 || len(filesystem) < 3 {
			return nil, fmt.Errorf("/proc/self/mountinfo: malformed entry")
		}
		if v2 && filesystem[0] != "cgroup2" || !v2 && (filesystem[0] != "cgroup" || !hasController(filesystem[2], "memory")) {
			continue
		}
		unescape := strings.NewReplacer("\\040", " ", "\\011", "\t", "\\012", "\n", "\\134", "\\")
		root, err := cgroupPath(unescape.Replace(fields[3]))
		if err != nil {
			return nil, fmt.Errorf("/proc/self/mountinfo root: %w", err)
		}
		point, err := cgroupPath(unescape.Replace(fields[4]))
		if err != nil {
			return nil, fmt.Errorf("/proc/self/mountinfo mount point: %w", err)
		}
		mounts = append(mounts, cgroupMount{root: root, point: point})
	}
	return mounts, nil
}

func cgroupPath(value string) (string, error) {
	if !path.IsAbs(value) {
		return "", fmt.Errorf("non-absolute cgroup path %q", value)
	}
	for _, part := range strings.Split(value, "/") {
		if part == ".." {
			return "", fmt.Errorf("cgroup path %q is outside the visible namespace", value)
		}
	}
	return path.Clean(value), nil
}

func hasController(controllers, name string) bool {
	for _, controller := range strings.Split(controllers, ",") {
		if controller == name {
			return true
		}
	}
	return false
}

func restrictHierarchy(status *Status, dir string, mount cgroupMount, v2 bool) error {
	limitName, usageName := "memory.limit_in_bytes", "memory.usage_in_bytes"
	if v2 {
		limitName, usageName = "memory.max", "memory.current"
	}
	for {
		limitPath := path.Join(dir, limitName)
		data, err := os.ReadFile(limitPath)
		// 只有统一层级真正的根没有 memory.max；非根缺失不能当成无限额。
		rootWithoutLimit := v2 && mount.root == "/" && dir == mount.point && os.IsNotExist(err)
		if err != nil && !rootWithoutLimit {
			return fmt.Errorf("read memory limit: %w", err)
		}
		if err == nil {
			value := strings.TrimSpace(string(data))
			unlimited := value == "max" || !v2 && value == "-1"
			var limit uint64
			if !unlimited {
				limit, err = strconv.ParseUint(value, 10, 64)
				if err != nil {
					return fmt.Errorf("parse %s: %w", limitPath, err)
				}
				// v1 的无限额值为按页对齐的 LONG_MAX，也兼容更大的 sentinel。
				unlimited = !v2 && limit >= uint64(1<<63)-uint64(os.Getpagesize())
			}
			if !unlimited {
				status.Total = min(status.Total, limit)
				usagePath := path.Join(dir, usageName)
				data, err = os.ReadFile(usagePath)
				if err != nil {
					return fmt.Errorf("read usage for finite memory limit: %w", err)
				}
				usage, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
				if err != nil {
					return fmt.Errorf("parse %s for finite memory limit: %w", usagePath, err)
				}
				var available uint64
				if usage < limit {
					available = limit - usage
				}
				status.Available = min(status.Available, available, status.Total)
			}
		}
		if dir == mount.point {
			return nil
		}
		dir = path.Dir(dir)
	}
}
