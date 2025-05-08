// Package orasclient provides functionality to interact with OCI (Open Container Initiative) registries
// using the ORAS (OCI Registry As Storage) library. This package allows for retrieving artifacts
// from OCI registries, getting their digests, and extracting their contents.
package orasclient

import (
	"context"
	"fmt"
	"github.com/opencontainers/go-digest"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
	"oras.land/oras-go/v2/registry/remote/retry"
	"os"
	"path/filepath"
)

// Filemap represents the contents of an OCI artifact.
// It contains the artifact's digest (a unique identifier) and a map of files
// where keys are filenames and values are the file contents as byte slices.
type Filemap struct {
	// Digest is the unique identifier of the artifact in the OCI registry
	Digest digest.Digest
	// Files is a map of filename to file content
	Files map[string][]byte
}

// CreateClient creates and configures a connection to an OCI registry repository.
//
// Parameters:
//   - registry: The address of the OCI registry (e.g., "docker.io/myorg/myrepo")
//   - creds: Docker credentials in JSON format for authentication, or empty for anonymous access
//
// Returns:
//   - A configured registry.Repository object that can be used to interact with the registry
//
// The function sets up authentication if credentials are provided, otherwise it configures
// for anonymous access. It uses retry mechanisms and authentication caching for better performance.
func CreateClient(registry string, creds []byte) registry.Repository {
	repo, err := remote.NewRepository(registry)
	if err != nil {
		panic(err)
	}

	if len(creds) > 0 {
		// prepare authentication using Docker credentials
		credStore, err := credentials.NewMemoryStoreFromDockerConfig(creds)
		if err != nil {
			panic(err)
		}
		// Note: The below code can be omitted if authentication is not required
		repo.Client = &auth.Client{
			Client:     retry.DefaultClient,
			Cache:      auth.NewCache(),
			Credential: credentials.Credential(credStore),
		}
	} else {
		// Configure for anonymous access
		repo.Client = &auth.Client{
			Client: retry.DefaultClient,
			Cache:  auth.NewCache(),
		}
	}
	return repo
}

// GetDigest retrieves the content digest (a unique identifier) of an artifact from an OCI registry.
//
// Parameters:
//   - registry: The address of the OCI registry (e.g., "docker.io/myorg/myrepo")
//   - tag: The tag or reference of the artifact to fetch
//   - creds: Docker credentials in JSON format for authentication, or empty for anonymous access
//
// Returns:
//   - A string representation of the artifact's digest (e.g., "sha256:1234abcd...")
//
// This function is useful for determining if an artifact has changed by comparing its digest
// with a previously stored value. The digest uniquely identifies the content of the artifact.
func GetDigest(registry string, tag string, creds []byte) string {
	// Create a client to connect to the registry
	repo := CreateClient(registry, creds)

	// Create a context for the operation
	ctx := context.Background()

	// Fetch just the manifest descriptor without downloading the entire artifact
	manifestDescriptor, _, err := oras.Fetch(ctx, repo, tag, oras.DefaultFetchOptions)
	if err != nil {
		panic(err)
	}

	// Return the string representation of the digest
	return manifestDescriptor.Digest.String()
}

// GetFiles downloads an artifact from an OCI registry and returns its contents as a Filemap.
//
// Parameters:
//   - registry: The address of the OCI registry (e.g., "docker.io/myorg/myrepo")
//   - tag: The tag or reference of the artifact to fetch
//   - creds: Docker credentials in JSON format for authentication, or empty for anonymous access
//
// Returns:
//   - A Filemap containing the artifact's digest and a map of its files
//
// This function performs several steps:
// 1. Creates a temporary directory to store the downloaded files
// 2. Sets up a file store using the ORAS library
// 3. Downloads the artifact from the registry to the temporary directory
// 4. Reads all files from the temporary directory into memory
// 5. Returns a Filemap with the artifact's digest and file contents
//
// The temporary directory is automatically cleaned up when the function returns.
func GetFiles(registy string, tag string, creds []byte) Filemap {
	// 1. Create a temporary directory to store the downloaded files
	tmpdir, err := os.MkdirTemp("/tmp", "oras")
	if err != nil {
		panic(err)
	}
	// Ensure the temporary directory is removed when the function returns
	defer os.RemoveAll(tmpdir)

	// 2. Create a file store using the ORAS library
	fs, err := file.New(tmpdir)
	if err != nil {
		panic(err)
	}
	defer fs.Close()

	// 3. Create a context and connect to the remote repository
	ctx := context.Background()
	repo := CreateClient(registy, creds)

	// 4. Download the artifact from the registry to the file store
	manifestDescriptor, err := oras.Copy(ctx, repo, tag, fs, tag, oras.DefaultCopyOptions)
	if err != nil {
		panic(err)
	}

	// 5. Read all files from the temporary directory into memory
	filesMap, err := GetFilesContentBinary(tmpdir)

	// 6. Return a Filemap with the artifact's digest and file contents
	return Filemap{
		Digest: manifestDescriptor.Digest,
		Files:  filesMap,
	}
}

// GetFilesContentBinary reads all files from a directory and returns their contents as a map.
//
// Parameters:
//   - dirPath: The path to the directory containing the files to read
//
// Returns:
//   - A map where keys are filenames and values are the file contents as byte slices
//   - An error if any file operations fail
//
// This function:
// 1. Lists all entries in the specified directory
// 2. Skips any subdirectories
// 3. Reads each file's content into memory
// 4. Creates a map with filenames as keys and file contents as values
//
// Note: Error messages are in German. They indicate directory reading errors or file reading errors.
func GetFilesContentBinary(dirPath string) (map[string][]byte, error) {
	// Initialize an empty map to store the file contents
	files := make(map[string][]byte)

	// Read all entries in the directory
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("fehler beim Lesen des Verzeichnisses: %v", err)
	}

	// Process each entry in the directory
	for _, entry := range entries {
		// Skip subdirectories
		if entry.IsDir() {
			continue
		}

		// Read the file content
		content, err := os.ReadFile(filepath.Join(dirPath, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("fehler beim Lesen der Datei %s: %v", entry.Name(), err)
		}

		// Add the file content to the map with the filename as the key
		files[entry.Name()] = content
	}

	return files, nil
}
