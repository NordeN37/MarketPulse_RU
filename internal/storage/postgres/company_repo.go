package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/NordeN37/MarketPulse_RU/internal/domain"
)

// CompanyRepo handles company persistence.
type CompanyRepo struct {
	db *DB
}

func NewCompanyRepo(db *DB) *CompanyRepo {
	return &CompanyRepo{db: db}
}

// GetAll returns all companies.
func (r *CompanyRepo) GetAll(ctx context.Context) ([]domain.Company, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, ticker, name, COALESCE(sector,''), country,
		       COALESCE(moex_id,''), COALESCE(smartlab_id,''),
		       COALESCE(market_cap,0), is_watchlist, created_at, updated_at
		FROM companies
		ORDER BY ticker`)
	if err != nil {
		return nil, fmt.Errorf("querying companies: %w", err)
	}
	defer rows.Close()

	var result []domain.Company
	for rows.Next() {
		var c domain.Company
		if err := rows.Scan(
			&c.ID, &c.Ticker, &c.Name, &c.Sector, &c.Country,
			&c.MoexID, &c.SmartlabID, &c.MarketCap, &c.IsWatchlist,
			&c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning company: %w", err)
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

// GetAllTickers returns all company ticker symbols from the database.
func (r *CompanyRepo) GetAllTickers(ctx context.Context) ([]string, error) {
	rows, err := r.db.Pool.Query(ctx, `SELECT ticker FROM companies ORDER BY ticker`)
	if err != nil {
		return nil, fmt.Errorf("querying tickers: %w", err)
	}
	defer rows.Close()

	var tickers []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("scanning ticker: %w", err)
		}
		tickers = append(tickers, t)
	}
	return tickers, rows.Err()
}

// GetByTicker finds a company by its ticker symbol.
func (r *CompanyRepo) GetByTicker(ctx context.Context, ticker string) (*domain.Company, error) {
	var c domain.Company
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, ticker, name, COALESCE(sector,''), country,
		       COALESCE(moex_id,''), COALESCE(smartlab_id,''),
		       COALESCE(market_cap,0), is_watchlist, created_at, updated_at
		FROM companies WHERE ticker = $1`, ticker).Scan(
		&c.ID, &c.Ticker, &c.Name, &c.Sector, &c.Country,
		&c.MoexID, &c.SmartlabID, &c.MarketCap, &c.IsWatchlist,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting company by ticker: %w", err)
	}
	return &c, nil
}

// GetByID retrieves a company by ID.
func (r *CompanyRepo) GetByID(ctx context.Context, id int64) (*domain.Company, error) {
	var c domain.Company
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, ticker, name, COALESCE(sector,''), country,
		       COALESCE(moex_id,''), COALESCE(smartlab_id,''),
		       COALESCE(market_cap,0), is_watchlist, created_at, updated_at
		FROM companies WHERE id = $1`, id).Scan(
		&c.ID, &c.Ticker, &c.Name, &c.Sector, &c.Country,
		&c.MoexID, &c.SmartlabID, &c.MarketCap, &c.IsWatchlist,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting company by id: %w", err)
	}
	return &c, nil
}

// GetBySector returns all companies in a given sector.
func (r *CompanyRepo) GetBySector(ctx context.Context, sector string) ([]domain.Company, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, ticker, name, COALESCE(sector,''), country,
		       COALESCE(moex_id,''), COALESCE(smartlab_id,''),
		       COALESCE(market_cap,0), is_watchlist, created_at, updated_at
		FROM companies WHERE sector = $1
		ORDER BY market_cap DESC`, sector)
	if err != nil {
		return nil, fmt.Errorf("querying companies by sector: %w", err)
	}
	defer rows.Close()

	var result []domain.Company
	for rows.Next() {
		var c domain.Company
		if err := rows.Scan(
			&c.ID, &c.Ticker, &c.Name, &c.Sector, &c.Country,
			&c.MoexID, &c.SmartlabID, &c.MarketCap, &c.IsWatchlist,
			&c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning company: %w", err)
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

// SearchByName performs a case-insensitive search by company name or ticker.
func (r *CompanyRepo) SearchByName(ctx context.Context, query string) ([]domain.Company, error) {
	pattern := "%" + query + "%"
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, ticker, name, COALESCE(sector,''), country,
		       COALESCE(moex_id,''), COALESCE(smartlab_id,''),
		       COALESCE(market_cap,0), is_watchlist, created_at, updated_at
		FROM companies
		WHERE name ILIKE $1 OR ticker ILIKE $1
		ORDER BY market_cap DESC
		LIMIT 20`, pattern)
	if err != nil {
		return nil, fmt.Errorf("searching companies: %w", err)
	}
	defer rows.Close()

	var result []domain.Company
	for rows.Next() {
		var c domain.Company
		if err := rows.Scan(
			&c.ID, &c.Ticker, &c.Name, &c.Sector, &c.Country,
			&c.MoexID, &c.SmartlabID, &c.MarketCap, &c.IsWatchlist,
			&c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning company: %w", err)
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

// Upsert inserts or updates a company.
func (r *CompanyRepo) Upsert(ctx context.Context, c *domain.Company) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO companies (ticker, name, sector, country, moex_id, smartlab_id, market_cap, is_watchlist)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (ticker) DO UPDATE SET
			name = EXCLUDED.name,
			sector = EXCLUDED.sector,
			moex_id = EXCLUDED.moex_id,
			smartlab_id = EXCLUDED.smartlab_id,
			market_cap = EXCLUDED.market_cap,
			updated_at = NOW()`,
		c.Ticker, c.Name, c.Sector, c.Country,
		c.MoexID, c.SmartlabID, c.MarketCap, c.IsWatchlist,
	)
	return err
}
