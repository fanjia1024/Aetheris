package object

// File 对象存储中的文件元数据与路径辅助（设计 struct.md 3.6 file.go）
type File struct {
	Path     string
	Size     int64
	Metadata map[string]string
}

// Key 返回用于存储的唯一键（通常即 Path）
func (f *File) Key() string {
	return f.Path
}
