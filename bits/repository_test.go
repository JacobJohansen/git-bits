package bits_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	mrand "math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nerdalize/git-bits/bits"
)

func GitInitRemote(t *testing.T) (dir string) {
	dir, err := ioutil.TempDir("", "test_remote_")
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("git", "init", "--bare")
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		t.Fatal(err)
	}

	return dir
}

func GitCloneWorkspace(remote string, t *testing.T) (dir string, repo *bits.Repository) {
	dir, err := ioutil.TempDir("", "test_remote_")
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("git", "clone", remote, dir)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		t.Fatal(err)
	}

	repo, err = bits.NewRepository(dir)
	if err != nil {
		t.Fatal(err)
	}

	return dir, repo
}

func GitConfigure(t *testing.T, ctx context.Context, repo *bits.Repository, conf map[string]string) {
	for k, val := range conf {
		err := repo.Git(ctx, nil, nil, "config", "--local", k, val)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func WriteGitAttrFile(t *testing.T, dir string, attr map[string]string) {
	f, err := os.Create(filepath.Join(dir, ".gitattributes"))
	if err != nil {
		t.Fatal(err)
	}

	defer f.Close()
	for pattern, attr := range attr {
		fmt.Fprintf(f, "%s\t%s\n", pattern, attr)
	}
}

func BuildBinaryInPath(t *testing.T, ctx context.Context) {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		t.Fatalf("GOPATH not set for building git-bits for integration test, env: %+v", os.Environ())
	}

	cmd := exec.CommandContext(ctx, "go", "build", "-o", filepath.Join(gopath, "bin", "git-bits"))
	cmd.Dir = filepath.Join(gopath, "src", "github.com", "nerdalize", "git-bits")
	err := cmd.Run()
	if err != nil {
		t.Fatalf("failed to build git-bits, make sure this project is in $GOPATH/src/github.com/nerdalize/nerdalize: %v", err)
	}

}

func WriteRandomFile(t *testing.T, path string, size int64) (f *os.File) {
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	randr := io.LimitReader(rand.Reader, size)
	_, err = io.Copy(f, randr)
	if err != nil {
		t.Fatal(err)
	}

	return f
}

func TestNewRepository(t *testing.T) {
	_, err := bits.NewRepository("/tmp/my-bogus-repo")
	if err == nil {
		t.Errorf("creating repo in non-existing directory should fail")
	} else {
		if !strings.Contains(err.Error(), "workspace") {
			t.Errorf("creating repo should fail with non existing dir error, got: %v", err)
		}
	}

	tdir, _ := ioutil.TempDir("", "test_wdir_")
	_, err = bits.NewRepository(tdir)
	if err == nil {
		t.Errorf("creating repo in non-git directory should fail")
	} else {
		if !strings.Contains(err.Error(), "workspace") {
			t.Errorf("creating repo should fail with exit code, got: %v", err)
		}
	}
}

func TestSplitCombineScan(t *testing.T) {
	ctx := context.Background()
	ctx, _ = context.WithTimeout(ctx, time.Second*10)

	BuildBinaryInPath(t, ctx) //@TODO this is terrible for unit testing

	remote1 := GitInitRemote(t)
	wd1, repo1 := GitCloneWorkspace(remote1, t)
	WriteGitAttrFile(t, wd1, map[string]string{
		"*.bin": "filter=bits",
	})

	GitConfigure(t, ctx, repo1, map[string]string{
		"filter.bits.clean":    "git bits split",
		"filter.bits.smudge":   "git bits combine",
		"filter.bits.required": "true",
	})

	fpath := filepath.Join(wd1, "file1.bin")
	f1 := WriteRandomFile(t, fpath, 5*1024*1024)
	f1.Close()

	err := repo1.Git(ctx, nil, nil, "add", "-A")
	if err != nil {
		t.Error(err)
	}

	err = repo1.Git(ctx, nil, nil, "commit", "-m", "c0")
	if err != nil {
		t.Error(err)
	}

	c0buf := bytes.NewBuffer(nil)
	err = repo1.Git(ctx, nil, c0buf, "rev-parse", "HEAD")
	if err != nil {
		t.Error(err)
	}

	c0 := strings.TrimSpace(c0buf.String())
	originalContent := bytes.NewBuffer(nil)

	f2, err := os.OpenFile(fpath, os.O_RDWR, 0666)
	if err != nil {
		t.Error(err)
	}

	_, err = io.Copy(originalContent, f2)
	if err != nil {
		t.Error(err)
	}

	_, err = f2.WriteAt([]byte{0x00}, 5)
	if err != nil {
		t.Error(err)
	}

	f2.Close()

	err = repo1.Git(ctx, nil, nil, "add", "-A")
	if err != nil {
		t.Error(err)
	}

	err = repo1.Git(ctx, nil, nil, "commit", "-m", "c1")
	if err != nil {
		t.Error(err)
	}

	c1buf := bytes.NewBuffer(nil)
	err = repo1.Git(ctx, nil, c1buf, "rev-parse", "HEAD")
	if err != nil {
		t.Error(err)
	}

	c1 := strings.TrimSpace(c1buf.String())

	err = repo1.Git(ctx, nil, nil, "checkout", c0)
	if err != nil {
		t.Error(err)
	}

	newContent, err := ioutil.ReadFile(fpath)
	if err != nil {
		t.Error(err)
	}

	if !bytes.Equal(originalContent.Bytes(), newContent) {
		t.Error("after checkout, file content should be equal to content before edit")
	}

	scanbuf := bytes.NewBuffer(nil)
	err = repo1.Scan(c0, c1, scanbuf)
	if err != nil {
		t.Error(err)
	}

	if len(scanbuf.Bytes())%(hex.EncodedLen(bits.KeySize)+1) != 0 {
		t.Errorf("expected a multitude keys to be returned but got: %s", scanbuf.String())
	}
}

func TestPushFetch(t *testing.T) {
	ctx := context.Background()
	ctx, _ = context.WithTimeout(ctx, time.Second*60)

	remote1 := GitInitRemote(t)
	wd1, repo1 := GitCloneWorkspace(remote1, t)
	WriteGitAttrFile(t, wd1, map[string]string{
		"*.bin": "filter=bits",
	})

	GitConfigure(t, ctx, repo1, map[string]string{
		"filter.bits.clean":    "git bits split",
		"filter.bits.smudge":   "git bits combine",
		"filter.bits.required": "true",
	})

	fsize := int64(5 * 1024 * 1024)
	fpath := filepath.Join(wd1, "file_a.bin")
	f1 := WriteRandomFile(t, fpath, fsize)
	f1.Close()

	err := repo1.Git(ctx, nil, nil, "add", "-A")
	if err != nil {
		t.Fatal(err)
	}

	err = repo1.Git(ctx, nil, nil, "commit", "-m", "base")
	if err != nil {
		t.Fatal(err)
	}

	err = repo1.Git(ctx, nil, nil, "push")
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		func() {
			f, err := os.OpenFile(fpath, os.O_RDWR, 0666)
			if err != nil {
				t.Fatal(err)
			}

			defer f.Close()
			pos := mrand.Int63n(fsize)
			_, err = f.WriteAt([]byte{0x01}, pos)
			if err != nil {
				t.Fatal(err)
			}

			err = repo1.Git(ctx, nil, nil, "add", "-A")
			if err != nil {
				t.Fatal(err)
			}

			err = repo1.Git(ctx, nil, nil, "commit", "-m", fmt.Sprintf("c%d", i))
			if err != nil {
				t.Fatal(err)
			}
		}()
	}

	buf := bytes.NewBuffer(nil)
	err = repo1.Git(ctx, nil, buf, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}

	localSha1 := strings.TrimSpace(buf.String())
	if localSha1 == "" {
		t.Fatal("expected local sha not to be empty")
	}

	buf = bytes.NewBuffer(nil)
	err = repo1.Git(ctx, nil, buf, "ls-remote", "origin", "HEAD")
	if err != nil {
		t.Fatal(err)
	}

	remoteRef := strings.Fields(buf.String())
	remoteSha1 := strings.TrimSpace(remoteRef[0])
	if remoteSha1 == "" {
		t.Fatal("expected remote sha not to be empty")
	}

	scanbuf := bytes.NewBuffer(nil)
	err = repo1.Scan(remoteSha1, localSha1, scanbuf)
	if err != nil {
		t.Error(err)
	}

	err = repo1.Push(scanbuf, "origin")
	if err != nil {
		t.Error(err)
	}

	wd2, repo2 := GitCloneWorkspace(remote1, t)
	WriteGitAttrFile(t, wd2, map[string]string{
		"*.bin": "filter=bits",
	})

	GitConfigure(t, ctx, repo2, map[string]string{
		"filter.bits.clean":    "git bits split",
		"filter.bits.smudge":   "git bits combine",
		"filter.bits.required": "true",
	})

}