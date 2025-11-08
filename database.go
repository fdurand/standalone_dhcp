package main

import (
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/inverse-inc/packetfence/go/log"
	_ "github.com/mattn/go-sqlite3"
	dhcp "github.com/krolaw/dhcp4"
)

var (
	db       *sql.DB
	dbMutex  sync.RWMutex
)

// DHCPOption represents a DHCP option override
type DHCPOption struct {
	OptionCode  int    `json:"option_code"`
	OptionValue string `json:"option_value"`
	OptionType  string `json:"option_type"` // ip, ips, string, uint32, uint16, uint8, hex
}

// OptionOverride represents a complete override entry
type OptionOverride struct {
	ID        int64        `json:"id"`
	Type      string       `json:"type"`       // "network" or "mac"
	Target    string       `json:"target"`     // network IP or MAC address
	Options   []DHCPOption `json:"options"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// InitDatabase initializes the SQLite database for option overrides
func InitDatabase(dbPath string) error {
	var err error
	db, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Create schema
	schema := `
	CREATE TABLE IF NOT EXISTS dhcp_option_overrides (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		type TEXT NOT NULL CHECK(type IN ('network', 'mac')),
		target TEXT NOT NULL,
		options TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(type, target)
	);

	CREATE INDEX IF NOT EXISTS idx_type_target ON dhcp_option_overrides(type, target);
	`

	_, err = db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	return nil
}

// CloseDatabase closes the database connection
func CloseDatabase() error {
	if db != nil {
		return db.Close()
	}
	return nil
}

// SaveOptionOverride saves or updates an option override
func SaveOptionOverride(overrideType, target string, options []DHCPOption) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	optionsJSON, err := json.Marshal(options)
	if err != nil {
		return fmt.Errorf("failed to marshal options: %w", err)
	}

	query := `
		INSERT INTO dhcp_option_overrides (type, target, options, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(type, target) DO UPDATE SET
			options = excluded.options,
			updated_at = CURRENT_TIMESTAMP
	`

	_, err = db.Exec(query, overrideType, target, string(optionsJSON))
	if err != nil {
		return fmt.Errorf("failed to save option override: %w", err)
	}

	return nil
}

// GetOptionOverride retrieves an option override
func GetOptionOverride(overrideType, target string) (*OptionOverride, error) {
	dbMutex.RLock()
	defer dbMutex.RUnlock()

	query := `
		SELECT id, type, target, options, created_at, updated_at
		FROM dhcp_option_overrides
		WHERE type = ? AND target = ?
	`

	var override OptionOverride
	var optionsJSON string

	err := db.QueryRow(query, overrideType, target).Scan(
		&override.ID,
		&override.Type,
		&override.Target,
		&optionsJSON,
		&override.CreatedAt,
		&override.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get option override: %w", err)
	}

	err = json.Unmarshal([]byte(optionsJSON), &override.Options)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal options: %w", err)
	}

	return &override, nil
}

// DeleteOptionOverride deletes an option override
func DeleteOptionOverride(overrideType, target string) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	query := `DELETE FROM dhcp_option_overrides WHERE type = ? AND target = ?`

	result, err := db.Exec(query, overrideType, target)
	if err != nil {
		return fmt.Errorf("failed to delete option override: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

// ListOptionOverrides lists all option overrides
func ListOptionOverrides(overrideType string) ([]OptionOverride, error) {
	dbMutex.RLock()
	defer dbMutex.RUnlock()

	var query string
	var args []interface{}

	if overrideType != "" {
		query = `
			SELECT id, type, target, options, created_at, updated_at
			FROM dhcp_option_overrides
			WHERE type = ?
			ORDER BY created_at DESC
		`
		args = append(args, overrideType)
	} else {
		query = `
			SELECT id, type, target, options, created_at, updated_at
			FROM dhcp_option_overrides
			ORDER BY type, created_at DESC
		`
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list option overrides: %w", err)
	}
	defer rows.Close()

	var overrides []OptionOverride

	for rows.Next() {
		var override OptionOverride
		var optionsJSON string

		err := rows.Scan(
			&override.ID,
			&override.Type,
			&override.Target,
			&optionsJSON,
			&override.CreatedAt,
			&override.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		err = json.Unmarshal([]byte(optionsJSON), &override.Options)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal options: %w", err)
		}

		overrides = append(overrides, override)
	}

	return overrides, nil
}

// ConvertOptionToDHCP converts a DHCPOption to dhcp.Options format
func ConvertOptionToDHCP(option DHCPOption) (dhcp.OptionCode, []byte, error) {
	code := dhcp.OptionCode(option.OptionCode)

	switch option.OptionType {
	case "ip":
		// Single IP address
		ip := net.ParseIP(option.OptionValue)
		if ip == nil {
			return 0, nil, fmt.Errorf("invalid IP address: %s", option.OptionValue)
		}
		ip4 := ip.To4()
		if ip4 == nil {
			return 0, nil, fmt.Errorf("not an IPv4 address: %s", option.OptionValue)
		}
		return code, []byte(ip4), nil

	case "ips":
		// Comma-separated IP addresses
		ipList := strings.Split(option.OptionValue, ",")
		var result []byte
		for _, ipStr := range ipList {
			ip := net.ParseIP(strings.TrimSpace(ipStr))
			if ip == nil {
				return 0, nil, fmt.Errorf("invalid IP address: %s", ipStr)
			}
			ip4 := ip.To4()
			if ip4 == nil {
				return 0, nil, fmt.Errorf("not an IPv4 address: %s", ipStr)
			}
			result = append(result, ip4...)
		}
		return code, result, nil

	case "string":
		// String value
		return code, []byte(option.OptionValue), nil

	case "uint32":
		// 32-bit unsigned integer
		val, err := strconv.ParseUint(option.OptionValue, 10, 32)
		if err != nil {
			return 0, nil, fmt.Errorf("invalid uint32: %s", option.OptionValue)
		}
		bytes := make([]byte, 4)
		binary.BigEndian.PutUint32(bytes, uint32(val))
		return code, bytes, nil

	case "uint16":
		// 16-bit unsigned integer
		val, err := strconv.ParseUint(option.OptionValue, 10, 16)
		if err != nil {
			return 0, nil, fmt.Errorf("invalid uint16: %s", option.OptionValue)
		}
		bytes := make([]byte, 2)
		binary.BigEndian.PutUint16(bytes, uint16(val))
		return code, bytes, nil

	case "uint8":
		// 8-bit unsigned integer
		val, err := strconv.ParseUint(option.OptionValue, 10, 8)
		if err != nil {
			return 0, nil, fmt.Errorf("invalid uint8: %s", option.OptionValue)
		}
		return code, []byte{byte(val)}, nil

	case "hex":
		// Hexadecimal string
		return code, []byte(option.OptionValue), nil

	default:
		return 0, nil, fmt.Errorf("unsupported option type: %s", option.OptionType)
	}
}

// ApplyOptionOverrides applies option overrides to DHCP options
func ApplyOptionOverrides(options dhcp.Options, networkIP, mac string) dhcp.Options {
	// Create a copy to avoid modifying the original
	result := make(dhcp.Options)
	for k, v := range options {
		result[k] = v
	}

	// Apply network-level overrides first
	if networkIP != "" {
		networkOverride, err := GetOptionOverride("network", networkIP)
		if err == nil && networkOverride != nil {
			for _, opt := range networkOverride.Options {
				code, value, err := ConvertOptionToDHCP(opt)
				if err != nil {
					log.LoggerWContext(ctx).Error(fmt.Sprintf("Failed to convert network option %d: %s", opt.OptionCode, err))
					continue
				}
				result[code] = value
			}
		}
	}

	// Apply MAC-level overrides (these take precedence)
	if mac != "" {
		macOverride, err := GetOptionOverride("mac", mac)
		if err == nil && macOverride != nil {
			for _, opt := range macOverride.Options {
				code, value, err := ConvertOptionToDHCP(opt)
				if err != nil {
					log.LoggerWContext(ctx).Error(fmt.Sprintf("Failed to convert MAC option %d: %s", opt.OptionCode, err))
					continue
				}
				result[code] = value
			}
		}
	}

	return result
}
