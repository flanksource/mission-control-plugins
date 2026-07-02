// S3 plugin: attaches to MissionControl::Connection catalog items for S3
// connections. It resolves the selected connection through
// HostClient.GetConnectionByID and exposes bucket browsing operations.
package main

import (
	"context"
	"crypto/tls"
	"embed"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	pluginpb "github.com/flanksource/incident-commander/plugin/api"
	"github.com/flanksource/incident-commander/plugin/sdk"
)

const (
	OpListBucket = "list-bucket"
	OpGetObject  = "get-object"
)

//go:embed all:ui
var uiAssets embed.FS

var (
	Version   = ""
	BuildDate = ""
)

func main() {
	serveAddr := flag.String("serve", "", "run as a standalone gRPC server on this address (e.g. :9000) instead of as a go-plugin subprocess")
	tlsCert := flag.String("serve-tls-cert", "", "TLS certificate file for --serve (enables TLS with --serve-tls-key)")
	tlsKey := flag.String("serve-tls-key", "", "TLS private key file for --serve")
	tlsClientCA := flag.String("serve-tls-client-ca", "", "PEM CA bundle to require and verify the host's client certificate (enables mTLS)")
	flag.Parse()

	sub, err := fs.Sub(uiAssets, "ui")
	if err != nil {
		panic(err)
	}

	plugin := newPlugin()
	if *serveAddr != "" {
		if (*tlsCert == "") != (*tlsKey == "") {
			fmt.Fprintln(os.Stderr, "s3: --serve-tls-cert and --serve-tls-key must be set together")
			os.Exit(1)
		}
		if *tlsClientCA != "" && *tlsCert == "" {
			fmt.Fprintln(os.Stderr, "s3: --serve-tls-client-ca requires --serve-tls-cert and --serve-tls-key")
			os.Exit(1)
		}
		opts := []sdk.Option{sdk.WithStaticAssets(sub)}
		if *tlsCert != "" || *tlsKey != "" {
			opts = append(opts, sdk.WithServerTLS(*tlsCert, *tlsKey))
		}
		if *tlsClientCA != "" {
			opts = append(opts, sdk.WithServerClientCA(*tlsClientCA))
		}
		if err := sdk.ServeGRPC(plugin, *serveAddr, opts...); err != nil {
			fmt.Fprintf(os.Stderr, "s3: serve grpc: %v\n", err)
			os.Exit(1)
		}
		return
	}

	sdk.Serve(plugin, sdk.WithStaticAssets(sub))
}

type S3Plugin struct{}

func newPlugin() *S3Plugin {
	return &S3Plugin{}
}

func (p *S3Plugin) Manifest() *pluginpb.PluginManifest {
	return &pluginpb.PluginManifest{
		Name:         "s3",
		Version:      sdk.FormatVersion(Version, BuildDate, ""),
		Description:  "Browse S3 buckets from Mission Control connection items.",
		Capabilities: []string{"tabs", "operations"},
		Tabs: []*pluginpb.TabSpec{
			{Name: "S3", Icon: "lucide:bucket", Path: "/", Scope: "config"},
		},
	}
}

func (p *S3Plugin) Configure(_ context.Context, _ map[string]any) error {
	return nil
}

func (p *S3Plugin) Operations() []sdk.Operation {
	return []sdk.Operation{
		{
			Def: &pluginpb.OperationDef{
				Name:        OpListBucket,
				Description: "List objects and metadata from the selected S3 bucket.",
				Scope:       "config",
				ResultMime:  sdk.ClickyResultMimeType,
				Http:        []*pluginpb.HTTPBinding{{Method: http.MethodPost}},
			},
			Handler:     p.listBucket,
			HTTPHandler: p.httpInvoke(OpListBucket, p.listBucket),
		},
		{
			Def: &pluginpb.OperationDef{
				Name:        OpGetObject,
				Description: "Read object metadata and a bounded preview of the selected S3 object.",
				Scope:       "config",
				ResultMime:  sdk.ClickyResultMimeType,
				Http:        []*pluginpb.HTTPBinding{{Method: http.MethodPost}},
			},
			Handler:     p.getObject,
			HTTPHandler: p.httpInvoke(OpGetObject, p.getObject),
		},
	}
}

type ListBucketParams struct {
	Prefix            string `json:"prefix,omitempty"`
	ContinuationToken string `json:"continuationToken,omitempty"`
	MaxKeys           int32  `json:"maxKeys,omitempty"`
	Delimiter         string `json:"delimiter,omitempty"`
}

type GetObjectParams struct {
	Key      string `json:"key"`
	Range    string `json:"range,omitempty"`
	MaxBytes int64  `json:"maxBytes,omitempty"`
}

type BucketListing struct {
	Bucket                string           `json:"bucket"`
	Prefix                string           `json:"prefix,omitempty"`
	Region                string           `json:"region,omitempty"`
	Endpoint              string           `json:"endpoint,omitempty"`
	UsePathStyle          bool             `json:"usePathStyle,omitempty"`
	CreatedAt             *time.Time       `json:"createdAt,omitempty"`
	ObjectCount           int              `json:"objectCount"`
	TotalSize             int64            `json:"totalSize"`
	IsTruncated           bool             `json:"isTruncated,omitempty"`
	NextContinuationToken string           `json:"nextContinuationToken,omitempty"`
	Prefixes              []PrefixMetadata `json:"prefixes,omitempty"`
	Objects               []ObjectMetadata `json:"objects"`
}

type PrefixMetadata struct {
	Name   string `json:"name"`
	Prefix string `json:"prefix"`
}

type ObjectMetadata struct {
	Name         string     `json:"name"`
	Key          string     `json:"key"`
	Size         int64      `json:"size"`
	CreatedAt    *time.Time `json:"createdAt,omitempty"`
	LastModified *time.Time `json:"lastModified,omitempty"`
	ETag         string     `json:"etag,omitempty"`
	StorageClass string     `json:"storageClass,omitempty"`
}

type ObjectContent struct {
	Name         string     `json:"name"`
	Key          string     `json:"key"`
	Size         int64      `json:"size,omitempty"`
	ContentType  string     `json:"contentType,omitempty"`
	LastModified *time.Time `json:"lastModified,omitempty"`
	ETag         string     `json:"etag,omitempty"`
	StorageClass string     `json:"storageClass,omitempty"`
	Content      string     `json:"content,omitempty"`
	Encoding     string     `json:"encoding,omitempty"`
	BytesRead    int64      `json:"bytesRead"`
	Truncated    bool       `json:"truncated,omitempty"`
	AcceptRanges string     `json:"acceptRanges,omitempty"`
	ContentRange string     `json:"contentRange,omitempty"`
}

type s3Connection struct {
	Client       *s3.Client
	Bucket       string
	Region       string
	Endpoint     string
	UsePathStyle bool
}

func (p *S3Plugin) httpInvoke(operation string, handler func(context.Context, sdk.InvokeCtx) (any, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { _ = r.Body.Close() }()
		params, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(strings.TrimSpace(string(params))) == 0 {
			params = []byte("{}")
		}

		res, err := handler(r.Context(), sdk.InvokeCtx{
			Operation:    operation,
			ParamsJSON:   params,
			ConfigItemID: sdk.ConfigItemIDFromContext(r.Context()),
			Host:         sdk.HostClientFromContext(r.Context()),
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(res); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}

func (p *S3Plugin) listBucket(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	if req.ConfigItemID == "" {
		return nil, fmt.Errorf("config_item_id is required")
	}
	if req.Host == nil {
		return nil, fmt.Errorf("host client is required")
	}

	var params ListBucketParams
	if len(req.ParamsJSON) > 0 {
		if err := json.Unmarshal(req.ParamsJSON, &params); err != nil {
			return nil, fmt.Errorf("decode params: %w", err)
		}
	}
	if params.MaxKeys <= 0 || params.MaxKeys > 1000 {
		params.MaxKeys = 1000
	}
	if params.Delimiter == "" && strings.HasSuffix(params.Prefix, "/") {
		params.Delimiter = "/"
	}

	conn, err := req.Host.GetConnectionByID(ctx, req.ConfigItemID)
	if err != nil {
		return nil, fmt.Errorf("get connection %s: %w", req.ConfigItemID, err)
	}
	resolved, err := buildS3Connection(ctx, conn)
	if err != nil {
		return nil, err
	}

	input := &s3.ListObjectsV2Input{
		Bucket:  aws.String(resolved.Bucket),
		MaxKeys: aws.Int32(params.MaxKeys),
	}
	if params.Prefix != "" {
		input.Prefix = aws.String(params.Prefix)
	}
	if params.ContinuationToken != "" {
		input.ContinuationToken = aws.String(params.ContinuationToken)
	}
	if params.Delimiter != "" {
		input.Delimiter = aws.String(params.Delimiter)
	}

	out, err := resolved.Client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("list s3 bucket %s: %w", resolved.Bucket, err)
	}

	listing := BucketListing{
		Bucket:                resolved.Bucket,
		Prefix:                params.Prefix,
		Region:                resolved.Region,
		Endpoint:              resolved.Endpoint,
		UsePathStyle:          resolved.UsePathStyle,
		CreatedAt:             bucketCreatedAt(ctx, resolved.Client, resolved.Bucket),
		IsTruncated:           aws.ToBool(out.IsTruncated),
		NextContinuationToken: aws.ToString(out.NextContinuationToken),
		Prefixes:              make([]PrefixMetadata, 0, len(out.CommonPrefixes)),
		Objects:               make([]ObjectMetadata, 0, len(out.Contents)),
	}

	for _, commonPrefix := range out.CommonPrefixes {
		prefix := aws.ToString(commonPrefix.Prefix)
		listing.Prefixes = append(listing.Prefixes, PrefixMetadata{
			Name:   prefixName(prefix),
			Prefix: prefix,
		})
	}

	for _, obj := range out.Contents {
		key := aws.ToString(obj.Key)
		size := aws.ToInt64(obj.Size)
		modified := utcPtr(obj.LastModified)
		listing.ObjectCount++
		listing.TotalSize += size
		listing.Objects = append(listing.Objects, ObjectMetadata{
			Name:         objectName(key),
			Key:          key,
			Size:         size,
			CreatedAt:    modified,
			LastModified: modified,
			ETag:         strings.Trim(aws.ToString(obj.ETag), `"`),
			StorageClass: string(obj.StorageClass),
		})
	}

	return listing, nil
}

func (p *S3Plugin) getObject(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	if req.ConfigItemID == "" {
		return nil, fmt.Errorf("config_item_id is required")
	}
	if req.Host == nil {
		return nil, fmt.Errorf("host client is required")
	}

	var params GetObjectParams
	if len(req.ParamsJSON) > 0 {
		if err := json.Unmarshal(req.ParamsJSON, &params); err != nil {
			return nil, fmt.Errorf("decode params: %w", err)
		}
	}
	if params.Key == "" {
		return nil, fmt.Errorf("key is required")
	}
	if params.MaxBytes <= 0 || params.MaxBytes > 5*1024*1024 {
		params.MaxBytes = 1024 * 1024
	}

	conn, err := req.Host.GetConnectionByID(ctx, req.ConfigItemID)
	if err != nil {
		return nil, fmt.Errorf("get connection %s: %w", req.ConfigItemID, err)
	}
	resolved, err := buildS3Connection(ctx, conn)
	if err != nil {
		return nil, err
	}

	input := &s3.GetObjectInput{
		Bucket: aws.String(resolved.Bucket),
		Key:    aws.String(params.Key),
	}
	if params.Range != "" {
		input.Range = aws.String(params.Range)
	}

	out, err := resolved.Client.GetObject(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("get s3 object %s/%s: %w", resolved.Bucket, params.Key, err)
	}
	defer func() {
		if err := out.Body.Close(); err != nil {
			log.Printf("failed to close body: %v", err)
		}
	}()

	data, err := io.ReadAll(io.LimitReader(out.Body, params.MaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read s3 object %s/%s: %w", resolved.Bucket, params.Key, err)
	}
	truncated := int64(len(data)) > params.MaxBytes
	if truncated {
		data = data[:params.MaxBytes]
	}

	encoding := "utf-8"
	content := string(data)
	if !isTextContent(aws.ToString(out.ContentType), data) {
		encoding = "base64"
		content = base64.StdEncoding.EncodeToString(data)
	}

	return ObjectContent{
		Name:         objectName(params.Key),
		Key:          params.Key,
		Size:         aws.ToInt64(out.ContentLength),
		ContentType:  aws.ToString(out.ContentType),
		LastModified: utcPtr(out.LastModified),
		ETag:         strings.Trim(aws.ToString(out.ETag), `"`),
		StorageClass: string(out.StorageClass),
		Content:      content,
		Encoding:     encoding,
		BytesRead:    int64(len(data)),
		Truncated:    truncated,
		AcceptRanges: aws.ToString(out.AcceptRanges),
		ContentRange: aws.ToString(out.ContentRange),
	}, nil
}

func buildS3Connection(ctx context.Context, conn *pluginpb.ResolvedConnection) (*s3Connection, error) {
	if conn == nil {
		return nil, fmt.Errorf("connection not found")
	}
	if conn.Type != "s3" {
		return nil, fmt.Errorf("s3 plugin requires an s3 connection, got %q", conn.Type)
	}

	props := map[string]any{}
	if conn.Properties != nil {
		props = conn.Properties.AsMap()
	}

	bucket := propertyString(props, "bucket")
	if bucket == "" {
		return nil, fmt.Errorf("s3 connection is missing bucket")
	}

	endpoint := conn.Url
	if endpoint == "" {
		endpoint = propertyString(props, "endpoint")
	}
	region := propertyString(props, "region")
	if region == "" && endpoint != "" {
		region = "us-east-1"
	}
	usePathStyle := propertyBool(props, "usePathStyle")
	insecureTLS := propertyBool(props, "insecureTLS")
	assumeRole := propertyString(props, "assumeRole")

	options := []func(*config.LoadOptions) error{}
	if region != "" {
		options = append(options, config.WithRegion(region))
	}
	if conn.Username != "" {
		options = append(options, config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(conn.Username, conn.Password, conn.Token)))
	}
	if insecureTLS {
		options = append(options, config.WithHTTPClient(&http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}))
	}

	cfg, err := config.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	if assumeRole != "" {
		cfg.Credentials = aws.NewCredentialsCache(stscreds.NewAssumeRoleProvider(sts.NewFromConfig(cfg), assumeRole))
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = usePathStyle
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
	})
	return &s3Connection{Client: client, Bucket: bucket, Region: region, Endpoint: endpoint, UsePathStyle: usePathStyle}, nil
}

func bucketCreatedAt(ctx context.Context, client *s3.Client, bucket string) *time.Time {
	out, err := client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil
	}
	for _, b := range out.Buckets {
		if aws.ToString(b.Name) == bucket {
			return utcPtr(b.CreationDate)
		}
	}
	return nil
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func objectName(key string) string {
	trimmed := strings.TrimSuffix(key, "/")
	if trimmed == "" {
		return key
	}
	name := path.Base(trimmed)
	if name == "." || name == "/" {
		return key
	}
	return name
}

func prefixName(prefix string) string {
	return objectName(strings.TrimSuffix(prefix, "/"))
}

func isTextContent(contentType string, data []byte) bool {
	contentType = strings.ToLower(contentType)
	if strings.HasPrefix(contentType, "text/") || strings.Contains(contentType, "json") || strings.Contains(contentType, "xml") || strings.Contains(contentType, "yaml") || strings.Contains(contentType, "javascript") {
		return true
	}
	return utf8.Valid(data)
}

func propertyString(props map[string]any, key string) string {
	if v, ok := props[key]; ok {
		switch t := v.(type) {
		case string:
			return t
		case fmt.Stringer:
			return t.String()
		}
	}
	return ""
}

func propertyBool(props map[string]any, key string) bool {
	if v, ok := props[key]; ok {
		switch t := v.(type) {
		case bool:
			return t
		case string:
			b, _ := strconv.ParseBool(t)
			return b
		}
	}
	return false
}
