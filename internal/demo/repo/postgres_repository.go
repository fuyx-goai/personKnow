package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	demo "knowledge-base/internal/demo/entity"
	documententity "knowledge-base/internal/document/entity"
	documentrepo "knowledge-base/internal/document/repo"
	platformpostgres "knowledge-base/internal/platform/postgres"
)

// PostgresRepository 负责把演示数据适配到生产使用的真实 PostgreSQL 表。
type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (repository *PostgresRepository) LatestActiveUser(ctx context.Context) (uuid.UUID, error) {
	var userID uuid.UUID
	err := repository.pool.QueryRow(ctx, `SELECT id FROM users WHERE status='active' AND deleted_at IS NULL
		ORDER BY last_login_at DESC NULLS LAST,created_at DESC LIMIT 1`).Scan(&userID)
	if err == pgx.ErrNoRows {
		return uuid.Nil, fmt.Errorf("没有可用的微信用户，请先在小程序完成登录")
	}
	return userID, err
}

// Seed 将原型数据写入真实表，并通过稳定主键保证重复执行幂等。
func (repository *PostgresRepository) Seed(ctx context.Context, userID uuid.UUID, dataDir string, additional ...[]demo.DocumentSeed) (demo.Report, error) {
	libraries, documents, sessions := demo.SeedData()
	for _, additions := range additional {
		documents = append(documents, additions...)
	}
	dataset := seedDataset{libraries: libraries, documents: documents, sessions: sessions}
	store := documentrepo.NewLocalFileStore(dataDir)
	prepared, err := repository.prepareFiles(ctx, store, seedFiles{userID: userID, libraries: libraries, documents: documents})
	if err != nil {
		return demo.Report{}, err
	}
	now := time.Now().UTC()
	err = platformpostgres.WithinTx(ctx, repository.pool, func(tx pgx.Tx) error {
		seed := seedTransaction{ctx: ctx, tx: tx, userID: userID, now: now}
		if err := ensurePlan(seed); err != nil {
			return fmt.Errorf("准备用户套餐失败: %w", err)
		}
		for _, library := range libraries {
			if err := upsertLibrary(seed, library); err != nil {
				return fmt.Errorf("写入知识库 %s 失败: %w", library.Key, err)
			}
		}
		for _, item := range prepared {
			if err := upsertDocument(seed, item); err != nil {
				return fmt.Errorf("写入文档 %s 失败: %w", item.seed.Key, err)
			}
		}
		if err := refreshLibraryStats(seed); err != nil {
			return fmt.Errorf("刷新知识库统计失败: %w", err)
		}
		for _, session := range sessions {
			if err := upsertSession(seed, session); err != nil {
				return fmt.Errorf("写入会话 %s 失败: %w", session.Key, err)
			}
		}
		if err := upsertUsage(seed, dataset); err != nil {
			return fmt.Errorf("写入用量失败: %w", err)
		}
		if err := upsertAudits(seed, dataset); err != nil {
			return fmt.Errorf("写入审计失败: %w", err)
		}
		return nil
	})
	if err != nil {
		return demo.Report{}, err
	}
	return demo.Report{UserID: userID, Libraries: len(libraries), Documents: len(documents), Sessions: len(sessions), UsageRecords: 4}, nil
}

type seedTransaction struct {
	ctx    context.Context
	tx     pgx.Tx
	userID uuid.UUID
	now    time.Time
}

type seedDataset struct {
	libraries []demo.LibrarySeed
	documents []demo.DocumentSeed
	sessions  []demo.SessionSeed
}

func refreshLibraryStats(seed seedTransaction) error {
	_, err := seed.tx.Exec(seed.ctx, `UPDATE knowledge_bases k SET
		document_count=COALESCE((SELECT count(*) FROM documents d WHERE d.library_id=k.id AND d.deleted_at IS NULL),0),
		chunk_count=COALESCE((SELECT sum(d.chunk_count) FROM documents d WHERE d.library_id=k.id AND d.deleted_at IS NULL),0),
		storage_bytes=COALESCE((SELECT sum(d.original_bytes) FROM documents d WHERE d.library_id=k.id AND d.deleted_at IS NULL),0),
		indexed_tokens=COALESCE((SELECT sum(d.indexed_tokens) FROM documents d WHERE d.library_id=k.id AND d.deleted_at IS NULL),0),updated_at=now()
		WHERE k.owner_user_id=$1 AND k.deleted_at IS NULL`, seed.userID)
	return err
}

type preparedDocument struct {
	seed         demo.DocumentSeed
	libraryID    uuid.UUID
	documentID   uuid.UUID
	originalPath string
	originalHash string
	originalSize int64
	contentPath  string
	contentHash  string
	contentSize  int64
	contentID    uuid.UUID
}

type seedFiles struct {
	userID    uuid.UUID
	libraries []demo.LibrarySeed
	documents []demo.DocumentSeed
}

func (repository *PostgresRepository) prepareFiles(ctx context.Context, store *documentrepo.LocalFileStore, files seedFiles) ([]preparedDocument, error) {
	libraryIDs := make(map[string]uuid.UUID, len(files.libraries))
	for _, library := range files.libraries {
		libraryIDs[library.Key] = demo.ID(files.userID, "library:"+library.Key)
	}
	prepared := make([]preparedDocument, 0, len(files.documents))
	for _, document := range files.documents {
		documentID := demo.ID(files.userID, "document:"+document.Key)
		location := documentLocation(files.userID, libraryIDs[document.LibraryKey], documentID)
		original, err := store.SaveOriginal(ctx, location, document.Name, strings.NewReader(document.Content), 50*1024*1024)
		if err != nil {
			return nil, err
		}
		content, err := store.WriteContent(location, 1, document.Content)
		if err != nil {
			return nil, err
		}
		prepared = append(prepared, preparedDocument{seed: document, libraryID: libraryIDs[document.LibraryKey], documentID: documentID,
			originalPath: original.RelativePath, originalHash: original.SHA256, originalSize: original.Size,
			contentPath: content.RelativePath, contentHash: content.SHA256, contentSize: content.Size,
			contentID: demo.ID(files.userID, "content:"+document.Key)})
	}
	return prepared, nil
}

func documentLocation(userID, libraryID, documentID uuid.UUID) documententity.Location {
	return documententity.Location{UserID: userID, LibraryID: libraryID, DocumentID: documentID}
}

func upsertLibrary(transaction seedTransaction, library demo.LibrarySeed) error {
	id := demo.ID(transaction.userID, "library:"+library.Key)
	_, err := transaction.tx.Exec(transaction.ctx, `INSERT INTO knowledge_bases(id,owner_user_id,name,category,description,visibility,status,created_at,updated_at)
		VALUES($1,$2,$3,$4,$5,'private','active',$6,$6)
		ON CONFLICT(id) DO UPDATE SET owner_user_id=$2,name=$3,category=$4,description=$5,visibility='private',status='active',deleted_at=NULL,updated_at=$6`,
		id, transaction.userID, library.Name, library.Category, library.Description, transaction.now)
	if err != nil {
		return err
	}
	_, err = transaction.tx.Exec(transaction.ctx, `INSERT INTO library_retrieval_settings(library_id,chunk_size,chunk_overlap,top_k,similarity_threshold,updated_by)
		VALUES($1,800,100,5,0.30,$2) ON CONFLICT(library_id) DO UPDATE SET updated_by=$2,updated_at=$3`, id, transaction.userID, transaction.now)
	return err
}

func upsertDocument(transaction seedTransaction, item preparedDocument) error {
	tags, err := json.Marshal(item.seed.Tags)
	if err != nil {
		return err
	}
	_, err = transaction.tx.Exec(transaction.ctx, `INSERT INTO documents(id,library_id,uploaded_by,original_name,display_name,extension,mime_type,original_bytes,original_sha256,original_path,status,summary,tags,chunk_count,indexed_tokens,created_at,updated_at)
		VALUES($1,$2,$3,$4,$4,'.md','text/markdown',$5,$6,$7,'ready',$8,$9,$10,$11,$12,$12)
		ON CONFLICT(id) DO UPDATE SET library_id=$2,uploaded_by=$3,display_name=$4,original_bytes=$5,original_sha256=$6,original_path=$7,
		status=CASE WHEN documents.original_sha256=$6 THEN documents.status ELSE 'ready' END,summary=$8,tags=$9,
		chunk_count=CASE WHEN documents.original_sha256=$6 THEN documents.chunk_count ELSE $10 END,
		indexed_tokens=CASE WHEN documents.original_sha256=$6 THEN documents.indexed_tokens ELSE $11 END,deleted_at=NULL,updated_at=$12`,
		item.documentID, item.libraryID, transaction.userID, item.seed.Name, item.originalSize, item.originalHash, item.originalPath,
		item.seed.Summary, tags, item.seed.ChunkCount, item.seed.IndexedToken, transaction.now)
	if err != nil {
		return err
	}
	structure := []byte(`{"source":"prototype-demo"}`)
	_, err = transaction.tx.Exec(transaction.ctx, `INSERT INTO document_contents(id,document_id,version,source_type,content_path,content_hash,text_bytes,token_estimate,structure_map,created_by,created_at)
		VALUES($1,$2,1,'parsed',$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT(document_id,version) DO UPDATE SET content_path=$3,content_hash=$4,text_bytes=$5,token_estimate=$6,structure_map=$7,created_by=$8`,
		item.contentID, item.documentID, item.contentPath, item.contentHash, item.contentSize, item.seed.IndexedToken, structure, transaction.userID, transaction.now)
	if err != nil {
		return err
	}
	_, err = transaction.tx.Exec(transaction.ctx, `UPDATE documents SET active_content_version_id=$2,updated_at=$3 WHERE id=$1`, item.documentID, item.contentID, transaction.now)
	if err != nil {
		return err
	}
	return scheduleSeedIndex(transaction, item)
}

// scheduleSeedIndex 使每个新内容哈希只创建一个索引任务，重复执行 seed 不会重复向量化。
func scheduleSeedIndex(transaction seedTransaction, item preparedDocument) error {
	jobID := demo.ID(transaction.userID, "index-job:"+item.seed.Key+":"+item.contentHash)
	result, err := transaction.tx.Exec(transaction.ctx, `INSERT INTO index_jobs
		(id,document_id,content_version_id,job_type,status,progress,stage,next_run_at,created_at,updated_at)
		VALUES($1,$2,$3,'index','queued',0,'queued',$4,$4,$4) ON CONFLICT(id) DO NOTHING`,
		jobID, item.documentID, item.contentID, transaction.now)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return nil
	}
	_, err = transaction.tx.Exec(transaction.ctx, `UPDATE documents SET status='queued',chunk_count=0,indexed_tokens=0,updated_at=$2 WHERE id=$1`, item.documentID, transaction.now)
	return err
}

func upsertSession(transaction seedTransaction, session demo.SessionSeed) error {
	libraryID := demo.ID(transaction.userID, "library:"+session.LibraryKey)
	sessionID := demo.ID(transaction.userID, "session:"+session.Key)
	_, err := transaction.tx.Exec(transaction.ctx, `INSERT INTO chat_sessions(id,user_id,scope_type,library_id,title,created_at,updated_at)
		VALUES($1,$2,'single_library',$3,$4,$5,$5) ON CONFLICT(id) DO UPDATE SET library_id=$3,title=$4,updated_at=$5,deleted_at=NULL`, sessionID, transaction.userID, libraryID, session.Title, transaction.now)
	if err != nil {
		return err
	}
	userMessageID := demo.ID(transaction.userID, "message:"+session.Key+":user")
	assistantMessageID := demo.ID(transaction.userID, "message:"+session.Key+":assistant")
	for _, message := range []struct {
		id            uuid.UUID
		role, content string
		input, output int64
	}{
		{userMessageID, "user", session.Question, session.InputTokens, 0},
		{assistantMessageID, "assistant", session.Answer, session.InputTokens, session.OutputTokens},
	} {
		_, err = transaction.tx.Exec(transaction.ctx, `INSERT INTO chat_messages(id,session_id,role,content,status,input_tokens,output_tokens,total_tokens,created_at,updated_at)
			VALUES($1,$2,$3,$4,'complete',$5,$6,$5::bigint+$6::bigint,$7,$7) ON CONFLICT(id) DO UPDATE SET content=$4,status='complete',input_tokens=$5,output_tokens=$6,total_tokens=$5::bigint+$6::bigint,updated_at=$7`,
			message.id, sessionID, message.role, message.content, message.input, message.output, transaction.now)
		if err != nil {
			return err
		}
	}
	refID := demo.ID(transaction.userID, "reference:"+session.Key)
	documentID := demo.ID(transaction.userID, "document:"+session.DocumentKey)
	contentID := demo.ID(transaction.userID, "content:"+session.DocumentKey)
	_, err = transaction.tx.Exec(transaction.ctx, `INSERT INTO message_references(id,message_id,document_id,content_version_id,chunk_id,source_name_snapshot,location_label,excerpt,similarity,rank)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,0.92,1) ON CONFLICT(id) DO UPDATE SET excerpt=$8,location_label=$7,similarity=0.92`, refID, assistantMessageID, documentID, contentID,
		"demo-"+session.DocumentKey, session.SourceName, session.Location, session.Excerpt)
	return err
}

func ensurePlan(transaction seedTransaction) error {
	_, err := transaction.tx.Exec(transaction.ctx, `INSERT INTO user_plans(id,user_id,plan_id)
		SELECT $1,$2,p.id FROM plans p WHERE p.code='personal_pro'
		AND NOT EXISTS (SELECT 1 FROM user_plans up WHERE up.user_id=$2 AND up.ends_at IS NULL)`, demo.ID(transaction.userID, "plan"), transaction.userID)
	return err
}

func upsertUsage(transaction seedTransaction, dataset seedDataset) error {
	month := time.Date(transaction.now.Year(), transaction.now.Month(), 1, 0, 0, 0, 0, time.UTC)
	storage := int64(0)
	for _, document := range dataset.documents {
		storage += int64(len(document.Content))
	}
	totalEmbedding := int64(0)
	for _, document := range dataset.documents {
		totalEmbedding += document.IndexedToken
	}
	totalInput, totalOutput := int64(0), int64(0)
	for _, session := range dataset.sessions {
		totalInput += session.InputTokens
		totalOutput += session.OutputTokens
	}
	_, err := transaction.tx.Exec(transaction.ctx, `INSERT INTO monthly_usage(user_id,month_start,storage_bytes,embedding_tokens,input_tokens,output_tokens,total_tokens)
		VALUES($1,$2,$3,$4,$5,$6,$4::bigint+$5::bigint+$6::bigint) ON CONFLICT(user_id,month_start) DO UPDATE SET
		storage_bytes=GREATEST(monthly_usage.storage_bytes,EXCLUDED.storage_bytes),embedding_tokens=GREATEST(monthly_usage.embedding_tokens,EXCLUDED.embedding_tokens),
		input_tokens=GREATEST(monthly_usage.input_tokens,EXCLUDED.input_tokens),output_tokens=GREATEST(monthly_usage.output_tokens,EXCLUDED.output_tokens),
		total_tokens=GREATEST(monthly_usage.total_tokens,EXCLUDED.total_tokens),updated_at=now()`, transaction.userID, month, storage, totalEmbedding, totalInput, totalOutput)
	if err != nil {
		return err
	}
	usage := []struct {
		key, usageType, unit, resourceType string
		quantity                           int64
		resourceID                         uuid.UUID
	}{
		{"storage", "storage", "bytes", "library", storage, demo.ID(transaction.userID, "library:"+dataset.libraries[0].Key)},
		{"embedding", "embedding", "tokens", "document", totalEmbedding, demo.ID(transaction.userID, "document:"+dataset.documents[0].Key)},
		{"input", "input", "tokens", "chat_session", totalInput, demo.ID(transaction.userID, "session:"+dataset.sessions[0].Key)},
		{"output", "output", "tokens", "chat_session", totalOutput, demo.ID(transaction.userID, "session:"+dataset.sessions[0].Key)},
	}
	for _, item := range usage {
		metadata := fmt.Sprintf(`{"seed_key":"%s"}`, item.key)
		if _, err := transaction.tx.Exec(transaction.ctx, `INSERT INTO usage_records(id,user_id,usage_type,quantity,unit,resource_type,resource_id,occurred_at,metadata)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(id) DO NOTHING`, demo.ID(transaction.userID, "usage:"+item.key), transaction.userID, item.usageType, item.quantity, item.unit, item.resourceType, item.resourceID, transaction.now, metadata); err != nil {
			return err
		}
	}
	return nil
}

func upsertAudits(transaction seedTransaction, dataset seedDataset) error {
	entries := []struct {
		key, action, resourceType string
		resourceID                uuid.UUID
	}{
		{"library", "library.create", "library", demo.ID(transaction.userID, "library:"+dataset.libraries[0].Key)},
		{"document", "document.index", "document", demo.ID(transaction.userID, "document:"+dataset.documents[0].Key)},
		{"chat", "chat.session.complete", "chat_session", demo.ID(transaction.userID, "session:rag-summary")},
	}
	for _, entry := range entries {
		metadata := fmt.Sprintf(`{"seed_key":"%s","source":"prototype-demo"}`, entry.key)
		if _, err := transaction.tx.Exec(transaction.ctx, `INSERT INTO audit_logs(actor_user_id,action,resource_type,resource_id,result,client_type,metadata,occurred_at)
			SELECT $1,$2::varchar,$3::varchar,$4::uuid,'success','miniprogram',$5::jsonb,$6 WHERE NOT EXISTS
			(SELECT 1 FROM audit_logs WHERE actor_user_id=$1 AND action=$2::varchar AND resource_id=$4::uuid AND metadata->>'seed_key'=$7::text)`,
			transaction.userID, entry.action, entry.resourceType, entry.resourceID, metadata, transaction.now, entry.key); err != nil {
			return err
		}
	}
	return nil
}
