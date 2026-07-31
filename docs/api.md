# I Owe You — API Documentation

**Project:** [github.com/matthewghuang/ioweyou](https://github.com/matthewghuang/ioweyou)  
**Description:** A group-expense tracking application backed by CRDT-based conflict-free replication. Supports offline-first operation with WebSocket-backed real-time sync between clients.

---

## Table of Contents

1. [Authentication](#1-authentication)
2. [Groups](#2-groups)
3. [Expenses](#3-expenses)
4. [Payments](#4-payments)
5. [Balances](#5-balances)
6. [Sync & Real-Time](#6-sync--real-time)
7. [CRDT Semantics & Behaviour](#7-crdt-semantics--behaviour)
8. [Error Responses](#8-error-responses)
9. [Data Model](#9-data-model)

---

## 1. Authentication

### 1.1 Register a User

Creates a new user and returns an API key. The API key is a 64-character hex string (32 random bytes). There is no login endpoint — the API key **is** the bearer token for all subsequent requests.

```
POST /api/auth/register
```

**Request Body:**

```json
{
  "name": "Alice"
}
```

**Response `201 Created`:**

```json
{
  "id": "uuid-string",
  "name": "Alice",
  "api_key": "a1b2c3d4e5f6..."
}
```

### 1.2 Authenticated Requests

All endpoints below require an `Authorization: Bearer <api_key>` header.

```
Authorization: Bearer a1b2c3d4e5f6...
```

On missing, invalid, or expired API key the server returns `401 Unauthorized`:

```json
{
  "error": "unauthorized"
}
```

---

## 2. Groups

Groups are the top-level container for expenses and payments. Each group has a name, a creator, and a membership roster (stored non-CRDT in `group_members` table). The creator is automatically added as a member.

### 2.1 Create Group

```
POST /api/groups
```

**Request Body:**

```json
{
  "name": "Ski Trip 2025"
}
```

**Response `201 Created`:**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "Ski Trip 2025",
  "created_by": "user-uuid",
  "created_at": 1719876543210000000,
  "members": ["user-uuid"]
}
```

**Behaviour:**
- Creator is automatically added as a group member.
- `created_at` is Unix nanosecond timestamp.
- `id` is auto-generated UUID.

### 2.2 List Groups

Returns all groups the authenticated user is a member of.

```
GET /api/groups
```

**Response `200 OK`:**

```json
[
  {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "name": "Ski Trip 2025",
    "created_by": "user-uuid",
    "created_at": 1719876543210000000,
    "members": ["user-uuid", "other-uuid"]
  }
]
```

### 2.3 Get Group

```
GET /api/groups/{id}
```

**Response `200 OK`:**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "Ski Trip 2025",
  "created_by": "user-uuid",
  "created_at": 1719876543210000000,
  "members": ["user-uuid", "other-uuid"]
}
```

**Errors:**
- `403` — Caller is not a group member.
- `404` — Group not found.

### 2.4 Update Group

Updates LWW (Last-Writer-Wins) fields on the group document. Any top-level key in the body is written as a new CRDT operation with the auth user's timestamp.

```
PATCH /api/groups/{id}
```

**Request Body:**

```json
{
  "name": "Ski Trip 2026"
}
```

**Response `200 OK`:**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "Ski Trip 2026",
  "created_by": "user-uuid",
  "created_at": 1719876543210000000,
  "members": ["user-uuid", "other-uuid"]
}
```

**Behaviour:**
- Every key-value pair in the request body becomes an LWW operation. The latest timestamp wins during merge.
- **Note:** Members cannot be added/removed via this endpoint — membership is managed through a direct SQL table (`group_members`).

---

## 3. Expenses

Expenses are transactions recorded inside a group. Each expense has a payer (`paid_by`), a total amount, and a list of per-person splits. Splits can be **equal** (auto-calculated across all group members) or **custom** (specified per-user).

### 3.1 Create Expense

```
POST /api/groups/{id}/expenses
```

**Request Body:**

```json
{
  "description": "Dinner",
  "amount": 100.00,
  "split_type": "equal",
  "splits": []
}
```

Or with custom splits:

```json
{
  "description": "Dinner",
  "amount": 100.00,
  "split_type": "custom",
  "splits": [
    { "user_id": "user-a-uuid", "amount": 50.00 },
    { "user_id": "user-b-uuid", "amount": 30.00 },
    { "user_id": "user-c-uuid", "amount": 20.00 }
  ]
}
```

**Response `201 Created`:**

```json
{
  "id": "expense-uuid",
  "group_id": "group-uuid",
  "description": "Dinner",
  "amount": 100.00,
  "paid_by": "user-a-uuid",
  "split_type": "equal",
  "created_at": 1719876543210000000,
  "splits": [
    { "user_id": "user-a-uuid", "amount": 25.00 },
    { "user_id": "user-b-uuid", "amount": 25.00 },
    { "user_id": "user-c-uuid", "amount": 25.00 },
    { "user_id": "user-d-uuid", "amount": 25.00 }
  ]
}
```

**Behaviour:**
- `paid_by` is always set to the authenticated user (the payer).
- `split_type` defaults to `"equal"` if omitted.
- **Equal split:** splits are auto-generated for every group member. The amount is `amount / len(members)`.
- **Custom split:** uses the `splits` array verbatim. No validation that amounts sum to the total.
- `splits` are stored as an RGA (Replicated Growable Array) list — ordering is preserved across sync.

**Errors:**
- `400` — Invalid body (empty description, non-positive amount, or invalid JSON).
- `403` — Caller is not a group member.

### 3.2 List Expenses in a Group

Returns all non-deleted expenses belonging to the group.

```
GET /api/groups/{id}/expenses
```

**Response `200 OK`:**

```json
[
  {
    "id": "expense-uuid",
    "group_id": "group-uuid",
    "description": "Dinner",
    "amount": 100.00,
    "paid_by": "user-uuid",
    "split_type": "equal",
    "created_at": 1719876543210000000,
    "splits": [...]
  }
]
```

**Behaviour:**
- Tombstoned (deleted) expenses are excluded.
- Returns an empty array (`[]`) when no expenses exist.

### 3.3 Get Expense

```
GET /api/expenses/{id}
```

**Response `200 OK`:** Same shape as a single expense in the list.

**Errors:**
- `403` — Caller is not a member of the expense's group.
- `404` — Expense not found or is tombstoned.

### 3.4 Update Expense

Updates fields and/or replaces the splits on an expense. Plain fields (description, amount, etc.) are updated with LWW semantics. If `"splits"` is present in the body, it **replaces** the entire split list via RGA delete-then-insert.

```
PATCH /api/expenses/{id}
```

**Request Body:**

```json
{
  "description": "Fancy Dinner",
  "amount": 120.00,
  "splits": [
    { "user_id": "user-a-uuid", "amount": 60.00 },
    { "user_id": "user-b-uuid", "amount": 60.00 }
  ]
}
```

**Response `200 OK`:** Returns the updated state.

**Behaviour:**
- Each scalar key (e.g. `description`, `amount`) is written as a new LWW CRDT operation. The version with the highest timestamp wins during merge.
- When `splits` is provided, existing RGA inserts are first deleted (tombstoned), then new inserts are appended. This is **not** a partial update — the entire list is replaced.
- The caller must be a member of the expense's group. They do **not** need to be the original payer.

### 3.5 Delete Expense

Soft-deletes (tombstones) the expense. It will no longer appear in list/get responses and is excluded from balance calculations.

```
DELETE /api/expenses/{id}
```

**Response `200 OK`:**

```json
{
  "status": "ok"
}
```

**Behaviour:**
- Writes an LWW `tombstone: true` operation. This is a CRDT operation, so a delete can be merged/conflicted like any other field.
- The expense data remains in the op log — it is not physically removed.

**Errors:**
- `403` — Caller is not a member of the expense's group.
- `404` — Expense not found or already tombstoned.

---

## 4. Payments

Payments record a settlement between two group members (a `from_user` paying a `to_user`). Payments start as `"pending"` and must be confirmed by the recipient.

### 4.1 Create Payment

```
POST /api/groups/{id}/payments
```

**Request Body:**

```json
{
  "from_user": "user-a-uuid",
  "to_user": "user-b-uuid",
  "amount": 50.00,
  "method": "venmo"
}
```

`method` is optional. Defaults to empty string.

**Response `201 Created`:**

```json
{
  "id": "payment-uuid",
  "group_id": "group-uuid",
  "from_user": "user-a-uuid",
  "to_user": "user-b-uuid",
  "amount": 50.00,
  "method": "venmo",
  "status": "pending",
  "created_at": 1719876543210000000
}
```

**Behaviour:**
- Status is always initialized to `"pending"`.
- The payment is tied to the group for balance calculation purposes.
- `method` is an informational string with no server-side validation.

**Errors:**
- `400` — `from_user`, `to_user`, or `amount` is missing/zero.
- `403` — Caller is not a group member.

### 4.2 List Payments in a Group

```
GET /api/groups/{id}/payments
```

**Response `200 OK`:**

```json
[
  {
    "id": "payment-uuid",
    "group_id": "group-uuid",
    "from_user": "user-a-uuid",
    "to_user": "user-b-uuid",
    "amount": 50.00,
    "method": "venmo",
    "status": "pending",
    "created_at": 1719876543210000000
  }
]
```

- Tombstoned (cancelled) payments are excluded.

### 4.3 Get Payment

```
GET /api/payments/{id}
```

**Response `200 OK`:** Same shape as a single payment in the list.

**Errors:**
- `403` — Caller is not a member of the payment's group.
- `404` — Not found or tombstoned.

### 4.4 Confirm Payment

Marks a pending payment as `"confirmed"`. **Only the recipient** (`to_user`) can confirm.

```
POST /api/payments/{id}/confirm
```

**No request body required.**

**Response `200 OK`:**

```json
{
  "id": "payment-uuid",
  "group_id": "group-uuid",
  "from_user": "user-a-uuid",
  "to_user": "user-b-uuid",
  "amount": 50.00,
  "method": "venmo",
  "status": "confirmed",
  "confirmed_at": 1719876633210000000,
  "confirmed_by": "user-b-uuid",
  "created_at": 1719876543210000000
}
```

**Behaviour:**
- Sets `status` → `"confirmed"`, `confirmed_at` → current nanosecond timestamp, `confirmed_by` → the confirming user's ID.
- Only confirmed payments affect balance calculations.

**Errors:**
- `400` — Payment is already confirmed.
- `403` — Caller is not the `to_user` (recipient) of the payment.
- `404` — Payment not found or tombstoned.

### 4.5 Cancel Payment

Soft-deletes (tombstones) a payment.

```
DELETE /api/payments/{id}
```

**Response `200 OK`:**

```json
{
  "status": "ok"
}
```

**Behaviour:**
- Same tombstone mechanism as expenses (`tombstone: true`).
- Any group member can cancel any payment within their group.

---

## 5. Balances

Returns a balance sheet with each member's net position, recorded payments, and recommended settlements with a per-expense breakdown.

### 5.1 Get Balances

```
GET /api/groups/{id}/balances
```

**Response `200 OK`:**

```json
{
  "members": [
    { "user_id": "alice-uuid", "balance": 50.00 },
    { "user_id": "bob-uuid",   "balance": 0.00 },
    { "user_id": "charlie-uuid", "balance": -25.00 },
    { "user_id": "dave-uuid",  "balance": -25.00 }
  ],
  "payments": [
    { "id": "pay-uuid", "from": "bob-uuid", "to": "alice-uuid", "amount": 25.00, "status": "confirmed", "method": "Venmo" }
  ],
  "settlements": [
    {
      "from": "charlie-uuid",
      "to": "alice-uuid",
      "amount": 25.00,
      "breakdown": [
        { "expense_name": "Dinner", "amount": 25.00 }
      ]
    },
    {
      "from": "dave-uuid",
      "to": "alice-uuid",
      "amount": 25.00,
      "breakdown": [
        { "expense_name": "Dinner", "amount": 25.00 }
      ]
    }
  ]
}
```

**Behaviour:**
1. **Expenses:** For each non-tombstoned expense, the payer is credited the full amount, and each participant is debited their split amount.
2. **Payments:** All non-tombstoned payments are collected and returned in the `payments` array. Only **confirmed** payments affect net balances: `from_user` is credited and `to_user` is debited (reducing the debt).
3. **Balance computation:** Debtors and creditors are sorted alphabetically and a greedy algorithm pairs them, producing the minimal number of recommended transfers.
4. Each settlement entry includes a `breakdown` array showing how each expense contributes to the transfer. Positive amounts mean the `from` user owes the `to` user for that expense; negative amounts mean the `to` user owes the `from` user for that expense (reducing the net owed).
5. Amounts are rounded to 2 decimal places.
6. Returns empty arrays (`{"members": [], "payments": [], "settlements": []}`) when there is no data.

**Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `members` | `BalanceMember[]` | Each member's net position. Positive = owed money, negative = owes money. |
| `payments` | `BalancePayment[]` | All recorded payments (confirmed and pending). |
| `settlements` | `BalanceEntry[]` | Recommended transfers to settle outstanding balances. Empty when everyone is settled. |

### BalanceMember

| Field | Type | Description |
|-------|------|-------------|
| `user_id` | string | User identifier |
| `balance` | float | Net balance. Positive = owed money, negative = owes money. |

### BalancePayment

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Payment document ID |
| `from` | string | Payer user ID |
| `to` | string | Recipient user ID |
| `amount` | float | Payment amount |
| `status` | string | `"confirmed"` or `"pending"` |
| `method` | string | Payment method (optional) |

**Example:**
- Alice pays $100 for dinner split 4 ways → Alice is owed $75, others owe $25 each.
- Bob pays Alice $25 via Venmo (confirmed) → Alice is owed $50, Bob owes $0, Charlie owes $25, Dave owes $25.
- Response contains members with their net positions, the payment record, and settlements for Charlie->Alice and Dave->Alice.

---

## 6. Sync & Real-Time

The sync layer enables offline-first operation and real-time collaboration. All data is stored as an append-only log of CRDT operations.

### 6.1 Pull Operations

Returns all operations newer than the client's last-seen cursors per author. Used to catch up after reconnecting.

```
POST /api/sync/pull
```

**Request Body:**

```json
{
  "client_id": "my-device-uuid",
  "doc_id": "550e8400-e29b-41d4-a716-446655440000",
  "cursors": {
    "author-user-a": { "wall_time": 1719876543210000000, "logical": 42 },
    "author-user-b": { "wall_time": 1719876500000000000, "logical": 17 }
  }
}
```

| Field | Description |
|-------|-------------|
| `client_id` | Unique identifier for this device/client. Used to persist per-client cursors on the server. |
| `doc_id` | Optional. If provided, only operations for this document are returned. If omitted, all operations across all documents are returned. |
| `cursors` | Per-author high-water marks. Operations with timestamps strictly after the cursor for their author are returned. Omit to get all operations. |

**Response `200 OK`:**

```json
{
  "operations": [...],
  "cursors": {
    "author-user-a": { "wall_time": 1719876543210000000, "logical": 43 },
    "author-user-b": { "wall_time": 1719876500000000000, "logical": 18 }
  }
}
```

### 6.2 Push Operations

Appends a batch of CRDT operations to the server. Duplicates are silently ignored (deduped by `UNIQUE(author_id, wall_time, logical)`).

```
POST /api/sync/push
```

**Request Body:**

```json
{
  "operations": [
    {
      "doc_id": "expense-uuid",
      "op_type": "lww",
      "field": "description",
      "value": "\"Dinner\"",
      "author_id": "user-uuid",
      "timestamp": { "wall_time": 1719876543210000000, "logical": 42 }
    }
  ]
}
```

**Response `200 OK`:**

```json
{
  "accepted": 1
}
```

**Behaviour:**
- Accepted operations are broadcast to all WebSocket subscribers of the relevant groups in real-time.
- The server advances its HLC by observing each received timestamp to maintain causal ordering.
- `accepted` indicates the count of operations successfully inserted (non-duplicates).

### 6.3 WebSocket Endpoint

Provides real-time push-and-subscribe for group changes. Use this for live UX instead of polling.

```
GET /api/ws?token=<api_key>
```

**Authentication:** Pass the API key as a query parameter (`token`). The standard `Authorization` header is **not** read for the WebSocket upgrade.

**WebSocket Protocol:**

**Client → Server messages:**

| Type | Purpose | Payload |
|------|---------|---------|
| `subscribe` | Subscribe to real-time changes for a group | `{"type":"subscribe","group_id":"group-uuid"}` |
| `push` | Push CRDT operations (same as REST push) | `{"type":"push","operations":[...]}` |

**Server → Client messages:**

| Type | Purpose | Payload |
|------|---------|---------|
| `change` | Notification of new operations for a subscribed group | `{"type":"change","operations":[...]}` |

**Ping/Pong:** The server sends a WebSocket ping every 30 seconds. Clients should respond with a pong.

**Behaviour:**
- On connect, the client should send `subscribe` messages for each group they are viewing.
- The server broadcasts newly pushed operations to all subscribers of the affected groups.
- Multiple groups can be subscribed to on a single connection.
- If a write to the WebSocket fails, the connection is unsubscribed and closed.

---

## 7. CRDT Semantics & Behaviour

The entire data model is built on an **append-only operation log** with two CRDT types:

### 7.1 Operation Types

| OpType | CRDT Strategy | Used For |
|--------|---------------|----------|
| `lww` | Last-Writer-Wins Register | Scalar fields: `name`, `description`, `amount`, `paid_by`, `status`, `tombstone` |
| `rga_insert` | RGA (Replicated Growable Array) — Insert | List entries: `splits` |
| `rga_delete` | RGA — Delete | Tombstoning list entries |

### 7.2 LWW (Last-Writer-Wins)

For scalar fields, the operation with the highest `(wall_time, logical, author_id)` wins during merge. This means:

- **Concurrent edits:** The most recent write (by wall clock + logical clock) wins. If timestamps are equal, the higher author_id (lexicographic) breaks the tie.
- **Tombstones:** Delete is just an LWW write setting `tombstone: true`. An undelete (setting `tombstone: false` with a higher timestamp) would revive the document.
- **Partial updates:** Each field evolves independently — updating `description` doesn't affect `amount`.

### 7.3 RGA (Replicated Growable Array)

Used for ordered lists (`splits`). Supports insert and delete operations:

- **Inserts** reference a `prev_item_id` for ordering. If the referenced item is deleted or unknown, the item is appended at the end.
- **Deletes** are speculative: a delete from one node only applies to inserts that node also knew about. A remote node that hasn't seen the insert won't apply the delete (safety).
- **Ordering** is deterministic and converges across peers by sorting on `(wall_time, logical, author_id, item_id)` and then reconstructing list order through `prev_item_id` references.

### 7.4 Snapshot Projection

The current state of any document is computed by:
1. Collecting all operations for that `doc_id`.
2. For each LWW field: keep the operation with the highest timestamp.
3. For each RGA field: sort surviving inserts (not deleted), order by `prev_item_id`, and unmarshal each value.

### 7.5 Merge Semantics

When two clients merge their operation logs (via pull/push or offline sync):

1. Operations are deduplicated by `(author_id, wall_time, logical)`.
2. LWW fields are resolved independently per `(doc_id, field)` — highest timestamp wins.
3. RGA deletions that originated locally only apply to inserts also known locally (speculative delete). Remote deletions apply to all matching inserts.
4. The result is a canonical, deterministic, conflict-free state.

### 7.6 Timestamps — Hybrid Logical Clock (HLC)

Each operation carries an HLC timestamp with two components:
- `wall_time`: Unix nanosecond wall clock time.
- `logical`: Monotonically increasing counter (per-node) to break ties within the same wall-time tick.

HLC ensures causal ordering across nodes: when a node receives a remote operation it advances its clock via `Observe()`, guaranteeing that future local operations have timestamps after the observed ones.

---

## 8. Error Responses

All errors return a JSON object with a single `"error"` string field.

| Status | Meaning | Typical Scenario |
|--------|---------|-----------------|
| `400` | Bad Request | Missing or invalid fields in request body. |
| `401` | Unauthorized | Missing/invalid `Authorization` header or WebSocket token. |
| `403` | Forbidden | Authenticated user is not a member of the group, or not allowed to perform the action (e.g. confirming a payment they don't receive). |
| `404` | Not Found | Document (group/expense/payment) doesn't exist or has been tombstoned. |
| `405` | Method Not Allowed | Wrong HTTP method (auth handler only). |
| `500` | Internal Server Error | Database query failure or unexpected server condition. |

**Error response shape:**

```json
{
  "error": "not a member"
}
```

**Success response shape:**

- `201` — `respondJSON(w, 201, data)` used for creates.
- `200` — `respondOK(w, data)` used for reads, updates, deletes. When `data` is `nil`, returns `{"status": "ok"}`.

---

## 9. Data Model

### 9.1 SQLite Tables

| Table | Purpose |
|-------|---------|
| `crdt_operations` | Append-only log of all CRDT mutations. The source of truth for all document state. |
| `users` | Registered users with ID, name, and API key. |
| `group_members` | Many-to-many group membership table (non-CRDT, direct SQL). |
| `sync_cursors` | Per-client per-author sync state for incremental pull. |

### 9.2 Document Types

**Group document** (stored in `crdt_operations`):

| Field | Type | CRDT |
|-------|------|------|
| `name` | string | LWW |
| `created_by` | string (user ID) | LWW |
| `created_at` | integer (Unix nanos) | LWW |

Membership is stored separately in the `group_members` table.

**Expense document:**

| Field | Type | CRDT |
|-------|------|------|
| `group_id` | string (UUID) | LWW |
| `description` | string | LWW |
| `amount` | number (float64) | LWW |
| `paid_by` | string (user ID) | LWW |
| `split_type` | string (`"equal"`/`"custom"`) | LWW |
| `created_at` | integer (Unix nanos) | LWW |
| `tombstone` | boolean | LWW |
| `splits` | array of `{user_id, amount}` | RGA |

**Payment document:**

| Field | Type | CRDT |
|-------|------|------|
| `group_id` | string (UUID) | LWW |
| `from_user` | string (user ID) | LWW |
| `to_user` | string (user ID) | LWW |
| `amount` | number (float64) | LWW |
| `method` | string | LWW |
| `status` | string (`"pending"`/`"confirmed"`) | LWW |
| `created_at` | integer (Unix nanos) | LWW |
| `confirmed_at` | integer (Unix nanos) | LWW |
| `confirmed_by` | string (user ID) | LWW |
| `tombstone` | boolean | LWW |

---

## Quick Reference — All Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/api/auth/register` | No | Register a user, get API key |
| `POST` | `/api/groups` | Yes | Create a group |
| `GET` | `/api/groups` | Yes | List my groups |
| `GET` | `/api/groups/{id}` | Yes | Get group details |
| `PATCH` | `/api/groups/{id}` | Yes | Update group fields |
| `POST` | `/api/groups/{id}/expenses` | Yes | Create expense |
| `GET` | `/api/groups/{id}/expenses` | Yes | List expenses |
| `GET` | `/api/expenses/{id}` | Yes | Get expense |
| `PATCH` | `/api/expenses/{id}` | Yes | Update expense |
| `DELETE` | `/api/expenses/{id}` | Yes | Delete expense |
| `POST` | `/api/groups/{id}/payments` | Yes | Create payment |
| `GET` | `/api/groups/{id}/payments` | Yes | List payments |
| `GET` | `/api/payments/{id}` | Yes | Get payment |
| `POST` | `/api/payments/{id}/confirm` | Yes | Confirm payment (recipient only) |
| `DELETE` | `/api/payments/{id}` | Yes | Cancel payment |
| `GET` | `/api/groups/{id}/balances` | Yes | Get settlement recommendations |
| `POST` | `/api/sync/pull` | Yes | Pull operations since cursors |
| `POST` | `/api/sync/push` | Yes | Push operations |
| `GET` | `/api/ws?token=` | Yes | WebSocket (token in query) |

---

*Generated from source at `github.com/matthewghuang/ioweyou`*
