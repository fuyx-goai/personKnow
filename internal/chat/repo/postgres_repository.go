package repo

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	. "knowledge-base/internal/chat/entity"
	platformpostgres "knowledge-base/internal/platform/postgres"
	platformvector "knowledge-base/internal/platform/vector"
	usage "knowledge-base/internal/usage/entity"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (repository *PostgresRepository) CreateSession(ctx context.Context, session Session) error {
	_, err := repository.pool.Exec(ctx, `INSERT INTO chat_sessions(id,user_id,scope_type,library_id,title,created_at,updated_at)
        VALUES($1,$2,$3,$4,$5,$6,$7)`, session.ID, session.UserID, session.ScopeType, session.LibraryID,
		session.Title, session.CreatedAt, session.UpdatedAt)
	return err
}

func (repository *PostgresRepository) GetSession(ctx context.Context, userID, sessionID uuid.UUID) (Session, error) {
	var session Session
	err := repository.pool.QueryRow(ctx, `SELECT id,user_id,scope_type,library_id,title,created_at,updated_at
        FROM chat_sessions WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL`, sessionID, userID).Scan(
		&session.ID, &session.UserID, &session.ScopeType, &session.LibraryID, &session.Title, &session.CreatedAt, &session.UpdatedAt)
	if err == pgx.ErrNoRows {
		return Session{}, ErrSessionNotFound
	}
	return session, err
}

func (repository *PostgresRepository) ListSessions(ctx context.Context, userID uuid.UUID, filter SessionFilter) ([]Session, error) {
	rows, err := repository.pool.Query(ctx, `SELECT id,user_id,scope_type,library_id,title,created_at,updated_at
        FROM chat_sessions WHERE user_id=$1 AND deleted_at IS NULL
        AND ($2::timestamptz IS NULL OR updated_at<$2) ORDER BY updated_at DESC,id DESC LIMIT $3`, userID, filter.Before, filter.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sessions := make([]Session, 0, filter.Limit)
	for rows.Next() {
		var session Session
		if err := rows.Scan(&session.ID, &session.UserID, &session.ScopeType, &session.LibraryID,
			&session.Title, &session.CreatedAt, &session.UpdatedAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func (repository *PostgresRepository) ResolveScope(ctx context.Context, userID uuid.UUID, session Session) (platformvector.SearchScope, RetrievalSettings, error) {
	query := `SELECT k.id,s.top_k,s.similarity_threshold FROM knowledge_bases k
        JOIN library_retrieval_settings s ON s.library_id=k.id
        WHERE k.deleted_at IS NULL AND k.status='active' AND (k.owner_user_id=$1 OR k.visibility='public')`
	arguments := []any{userID}
	if session.ScopeType == ScopeSingleLibrary {
		query += ` AND k.id=$2`
		arguments = append(arguments, session.LibraryID)
	}
	rows, err := repository.pool.Query(ctx, query, arguments...)
	if err != nil {
		return platformvector.SearchScope{}, RetrievalSettings{}, err
	}
	defer rows.Close()
	settings := RetrievalSettings{}
	var libraryIDs []uuid.UUID
	for rows.Next() {
		var libraryID uuid.UUID
		var topK int
		var threshold float64
		if err := rows.Scan(&libraryID, &topK, &threshold); err != nil {
			return platformvector.SearchScope{}, RetrievalSettings{}, err
		}
		libraryIDs = append(libraryIDs, libraryID)
		settings.TopK = max(settings.TopK, topK)
		if settings.SimilarityThreshold == 0 || threshold < settings.SimilarityThreshold {
			settings.SimilarityThreshold = threshold
		}
	}
	if err := rows.Err(); err != nil {
		return platformvector.SearchScope{}, RetrievalSettings{}, err
	}
	if session.ScopeType == ScopeSingleLibrary && len(libraryIDs) == 0 {
		return platformvector.SearchScope{}, RetrievalSettings{}, ErrSessionNotFound
	}
	active, err := repository.activeVersions(ctx, libraryIDs)
	scope := platformvector.SearchScope{LibraryIDs: libraryIDs, ActiveVersions: active, MinSimilarity: settings.SimilarityThreshold}
	return scope, settings, err
}

func (repository *PostgresRepository) activeVersions(ctx context.Context, libraryIDs []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	active := make(map[uuid.UUID]uuid.UUID)
	if len(libraryIDs) == 0 {
		return active, nil
	}
	rows, err := repository.pool.Query(ctx, `SELECT id,active_content_version_id FROM documents
        WHERE library_id=ANY($1) AND deleted_at IS NULL AND status='ready' AND active_content_version_id IS NOT NULL`, libraryIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var documentID, versionID uuid.UUID
		if err := rows.Scan(&documentID, &versionID); err != nil {
			return nil, err
		}
		active[documentID] = versionID
	}
	return active, rows.Err()
}

func (repository *PostgresRepository) StartExchange(ctx context.Context, session Session, question string, now time.Time) (Message, Message, error) {
	userMessage := Message{ID: uuid.New(), SessionID: session.ID, Role: RoleUser, Content: question, Status: MessageComplete, CreatedAt: now, UpdatedAt: now}
	assistant := Message{ID: uuid.New(), SessionID: session.ID, Role: RoleAssistant, Status: MessageStreaming, CreatedAt: now, UpdatedAt: now}
	err := platformpostgres.WithinTx(ctx, repository.pool, func(tx pgx.Tx) error {
		for _, message := range []Message{userMessage, assistant} {
			if _, err := tx.Exec(ctx, `INSERT INTO chat_messages(id,session_id,role,content,status,created_at,updated_at)
                VALUES($1,$2,$3,$4,$5,$6,$7)`, message.ID, message.SessionID, message.Role,
				message.Content, message.Status, message.CreatedAt, message.UpdatedAt); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `UPDATE chat_sessions SET updated_at=$2 WHERE id=$1`, session.ID, now)
		return err
	})
	return userMessage, assistant, err
}

func (repository *PostgresRepository) FinishExchange(ctx context.Context, assistant Message, answer string, status MessageStatus, breakdown usage.Breakdown, references []Reference, now time.Time) error {
	return platformpostgres.WithinTx(ctx, repository.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE chat_messages SET content=$2,status=$3,input_tokens=$4,output_tokens=$5,
            total_tokens=$6,updated_at=$7 WHERE id=$1`, assistant.ID, answer, status, breakdown.Input,
			breakdown.Output, breakdown.Total(), now)
		if err != nil {
			return err
		}
		for _, reference := range references {
			_, err := tx.Exec(ctx, `INSERT INTO message_references
                (id,message_id,document_id,content_version_id,chunk_id,source_name_snapshot,location_label,excerpt,similarity,rank)
                VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, uuid.New(), assistant.ID, reference.DocumentID,
				reference.ContentVersionID, reference.ChunkID, reference.SourceName, reference.LocationLabel,
				reference.Excerpt, reference.Similarity, reference.Rank)
			if err != nil {
				return err
			}
		}
		return nil
	})
}
