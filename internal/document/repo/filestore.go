package repo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	. "knowledge-base/internal/document/entity"
)

type LocalFileStore struct {
	root string
}

func NewLocalFileStore(root string) *LocalFileStore {
	return &LocalFileStore{root: filepath.Clean(root)}
}

func (store *LocalFileStore) SaveOriginal(ctx context.Context, location Location, originalName string, reader io.Reader, maxBytes int64) (StoredFile, error) {
	extension := strings.ToLower(filepath.Ext(filepath.Base(originalName)))
	directory := store.documentDir(location, "original")
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return StoredFile{}, err
	}
	temporary, err := os.CreateTemp(directory, ".upload-*.tmp")
	if err != nil {
		return StoredFile{}, err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(reader, maxBytes+1))
	closeErr := temporary.Close()
	if copyErr != nil {
		return StoredFile{}, copyErr
	}
	if closeErr != nil {
		return StoredFile{}, closeErr
	}
	if written > maxBytes {
		return StoredFile{}, ErrFileTooLarge
	}
	if err := ctx.Err(); err != nil {
		return StoredFile{}, err
	}
	fileName := "source" + extension
	finalPath := filepath.Join(directory, fileName)
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		return StoredFile{}, err
	}
	relative, err := filepath.Rel(store.root, finalPath)
	if err != nil {
		return StoredFile{}, err
	}
	return StoredFile{RelativePath: filepath.ToSlash(relative), Size: written, SHA256: hex.EncodeToString(hash.Sum(nil)), Extension: extension}, nil
}

func (store *LocalFileStore) WriteContent(location Location, version int, text string) (StoredFile, error) {
	directory := store.documentDir(location, "content")
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return StoredFile{}, err
	}
	path := filepath.Join(directory, fmt.Sprintf("v%d.txt", version))
	temporary := path + "." + uuid.NewString() + ".tmp"
	data := []byte(text)
	if err := os.WriteFile(temporary, data, 0o640); err != nil {
		return StoredFile{}, err
	}
	defer os.Remove(temporary)
	if err := os.Rename(temporary, path); err != nil {
		return StoredFile{}, err
	}
	hash := sha256.Sum256(data)
	relative, err := filepath.Rel(store.root, path)
	if err != nil {
		return StoredFile{}, err
	}
	return StoredFile{RelativePath: filepath.ToSlash(relative), Size: int64(len(data)), SHA256: hex.EncodeToString(hash[:]), Extension: ".txt"}, nil
}

func (store *LocalFileStore) Read(relativePath string) ([]byte, error) {
	path, err := store.safePath(relativePath)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func (store *LocalFileStore) DeleteDocument(location Location) error {
	return os.RemoveAll(store.documentDir(location))
}

func (store *LocalFileStore) Absolute(relativePath string) string {
	path, err := store.safePath(relativePath)
	if err != nil {
		return ""
	}
	return path
}

func (store *LocalFileStore) DetectFormat(path, originalName string) (Format, string, error) {
	return DetectFormat(path, originalName)
}

func (store *LocalFileStore) documentDir(location Location, parts ...string) string {
	base := filepath.Join(store.root, "users", location.UserID.String(), "libraries", location.LibraryID.String(), "documents", location.DocumentID.String())
	return filepath.Join(append([]string{base}, parts...)...)
}

func (store *LocalFileStore) safePath(relativePath string) (string, error) {
	path := filepath.Join(store.root, filepath.FromSlash(relativePath))
	relative, err := filepath.Rel(store.root, path)
	if err != nil || strings.HasPrefix(relative, "..") || filepath.IsAbs(relative) {
		return "", fmt.Errorf("文件路径越界")
	}
	return path, nil
}
