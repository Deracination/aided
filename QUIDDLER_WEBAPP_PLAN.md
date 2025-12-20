# Quiddler Multiplayer Web App - Implementation Plan

## 1. Project Overview

Reimplementation of the Quiddler card game as a real-time multiplayer web application. Quiddler is a word-building card game where players form words from letter cards across 8 rounds of increasing hand sizes (3-10 cards).

### Core Game Mechanics to Implement
- **Deck**: 118 cards with letters A-Z plus special double-letter cards (QU, IN, ER, TH, CL)
- **Rounds**: 8 rounds, starting with 3 cards, adding 1 each round up to 10
- **Turn Flow**: Draw from deck/discard → Form words → Discard one card
- **Going Out**: Player uses all cards in valid words, others get one more turn
- **Scoring**: Card point values, +10 bonus for most words, +10 for longest word
- **Word Validation**: Dictionary lookup, no proper nouns/prefixes/suffixes/abbreviations
- **Challenge System**: Players can challenge words for point penalties

---

## 2. Technology Stack

### Frontend
| Component | Technology | Rationale |
|-----------|------------|-----------|
| Framework | **React 18+** with TypeScript | Component-based, excellent ecosystem, type safety |
| State Management | **Zustand** or **Redux Toolkit** | Lightweight, handles complex game state |
| Styling | **Tailwind CSS** | Rapid UI development, responsive design |
| Real-time | **Socket.IO Client** | Robust WebSocket abstraction |
| Animations | **Framer Motion** | Smooth card animations, drag-and-drop |
| Build Tool | **Vite** | Fast development, optimized builds |

### Backend
| Component | Technology | Rationale |
|-----------|------------|-----------|
| Runtime | **Node.js** with TypeScript | Same language as frontend, async performance |
| Framework | **Express** or **Fastify** | Lightweight HTTP server |
| Real-time | **Socket.IO** | Rooms, namespaces, reconnection handling |
| Database | **PostgreSQL** | Relational data, user accounts, game history |
| ORM | **Prisma** | Type-safe queries, migrations |
| Cache | **Redis** | Session storage, game state cache, pub/sub |
| Auth | **Passport.js** + JWT | Flexible authentication strategies |

### Infrastructure
| Component | Technology | Rationale |
|-----------|------------|-----------|
| Hosting | **Railway** / **Render** / **Fly.io** | Easy deployment, WebSocket support |
| Database Hosting | **Supabase** or managed PostgreSQL | Managed, scalable |
| CDN | **Cloudflare** | Static assets, DDoS protection |

---

## 3. Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        FRONTEND (React)                         │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────────┐ │
│  │  Lobby   │  │Game Room │  │ Game UI  │  │ Card Components  │ │
│  │  System  │  │  Manager │  │  Board   │  │  Hand, Discard   │ │
│  └──────────┘  └──────────┘  └──────────┘  └──────────────────┘ │
│                         ▼                                        │
│  ┌──────────────────────────────────────────────────────────────┐│
│  │              Socket.IO Client + Zustand Store                 ││
│  └──────────────────────────────────────────────────────────────┘│
└───────────────────────────────┬─────────────────────────────────┘
                                │ WebSocket + REST
┌───────────────────────────────▼─────────────────────────────────┐
│                        BACKEND (Node.js)                         │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────────┐ │
│  │   Auth   │  │  Lobby   │  │   Game   │  │   Dictionary     │ │
│  │  Service │  │  Service │  │  Engine  │  │    Validator     │ │
│  └──────────┘  └──────────┘  └──────────┘  └──────────────────┘ │
│                         ▼                                        │
│  ┌────────────────┐  ┌────────────────┐  ┌─────────────────────┐│
│  │   PostgreSQL   │  │     Redis      │  │  Socket.IO Server  ││
│  │  (persistent)  │  │ (game state)   │  │     (rooms)        ││
│  └────────────────┘  └────────────────┘  └─────────────────────┘│
└─────────────────────────────────────────────────────────────────┘
```

---

## 4. Data Models

### Database Schema (PostgreSQL)

```typescript
// User
model User {
  id            String   @id @default(uuid())
  email         String   @unique
  username      String   @unique
  passwordHash  String
  avatar        String?
  gamesPlayed   Int      @default(0)
  gamesWon      Int      @default(0)
  totalScore    Int      @default(0)
  createdAt     DateTime @default(now())
  updatedAt     DateTime @updatedAt
}

// GameRecord (completed games for history)
model GameRecord {
  id          String   @id @default(uuid())
  players     Json     // [{userId, finalScore, placement}]
  rounds      Int      // number of rounds played
  winnerId    String
  playedAt    DateTime @default(now())
}
```

### In-Memory Game State (Redis/Memory)

```typescript
interface GameState {
  id: string;
  status: 'waiting' | 'in_progress' | 'finished';
  currentRound: number;        // 1-8
  currentPlayerIndex: number;
  turnPhase: 'draw' | 'play' | 'discard';

  players: Player[];
  deck: Card[];
  discardPile: Card[];

  roundGoingOut: boolean;      // someone went out
  goingOutPlayerIndex: number | null;
  lastTurnPlayers: Set<string>; // players who get final turn

  roundScores: RoundScore[][];  // per round, per player
}

interface Player {
  id: string;
  username: string;
  hand: Card[];
  laidDownWords: Card[][];     // words formed when going out
  totalScore: number;
  connected: boolean;
}

interface Card {
  id: string;
  letters: string;      // 'A', 'QU', 'TH', etc.
  points: number;
}
```

---

## 5. Core Features & Implementation Phases

### Phase 1: Foundation (Week 1-2)
- [ ] Project scaffolding (monorepo with Turborepo or separate repos)
- [ ] Basic Express/Fastify server with Socket.IO
- [ ] React app with routing (React Router)
- [ ] User authentication (register, login, JWT)
- [ ] Basic UI layout and styling
- [ ] Card component with point values

### Phase 2: Game Engine (Week 2-3)
- [ ] Card deck generation with correct distribution
- [ ] Game state machine (waiting → playing → finished)
- [ ] Turn logic (draw, form words, discard)
- [ ] Round progression (deal increasing cards)
- [ ] Going out detection
- [ ] Dictionary integration for word validation
- [ ] Scoring calculation with bonuses

### Phase 3: Multiplayer (Week 3-4)
- [ ] Lobby system (create/join rooms)
- [ ] Real-time game state synchronization
- [ ] Player turn management
- [ ] Reconnection handling
- [ ] Spectator mode (optional)
- [ ] Game invites via link

### Phase 4: Game UI (Week 4-5)
- [ ] Interactive hand display with drag-and-drop
- [ ] Word building area
- [ ] Discard pile and draw deck visualization
- [ ] Player status indicators
- [ ] Score board (current round + total)
- [ ] Round/game end summaries
- [ ] Card animations (deal, draw, discard)

### Phase 5: Polish & Deploy (Week 5-6)
- [ ] Challenge system for disputed words
- [ ] Sound effects (optional)
- [ ] Mobile responsiveness
- [ ] Error handling and edge cases
- [ ] Performance optimization
- [ ] Deployment and CI/CD
- [ ] Basic analytics/monitoring

---

## 6. Key Technical Decisions

### Dictionary/Word Validation
**Recommendation: Use a pre-compiled word list**

Options:
1. **SOWPODS / TWL** (Scrabble dictionaries) - Comprehensive, standard
2. **Datamuse API** - Online lookup, rate-limited
3. **Local Trie structure** - Fast in-memory lookups

**Implementation:**
```typescript
// Load dictionary into a Set for O(1) lookups
const dictionary = new Set<string>(wordListArray);

function isValidWord(word: string): boolean {
  const normalized = word.toLowerCase();
  if (normalized.length < 2) return false;
  return dictionary.has(normalized);
}
```

### Real-time Synchronization Strategy
**Server-authoritative model:**
1. Client sends actions (draw, place word, discard)
2. Server validates and updates game state
3. Server broadcasts new state to all players
4. Client renders based on received state

**Socket Events:**
```typescript
// Client → Server
'game:draw'         // { source: 'deck' | 'discard' }
'game:form_words'   // { words: Card[][] }
'game:discard'      // { cardId: string }
'game:challenge'    // { playerId: string, word: string }

// Server → Client
'game:state_update' // full or partial game state
'game:player_turn'  // whose turn it is
'game:round_end'    // round scores and bonuses
'game:end'          // final scores and winner
```

### Turn Timer (Optional Enhancement)
- Configurable turn time limit (e.g., 60-120 seconds)
- Warning at 15 seconds
- Auto-pass or auto-discard if timer expires

---

## 7. Card Distribution

Based on official Quiddler deck (118 cards):

| Letter | Count | Points | | Letter | Count | Points |
|--------|-------|--------|---|--------|-------|--------|
| A | 10 | 2 | | N | 6 | 2 |
| B | 2 | 8 | | O | 8 | 2 |
| C | 2 | 8 | | P | 2 | 6 |
| D | 4 | 5 | | Q | 2 | 15 |
| E | 12 | 2 | | R | 6 | 5 |
| F | 2 | 6 | | S | 4 | 3 |
| G | 4 | 6 | | T | 6 | 3 |
| H | 2 | 7 | | U | 6 | 4 |
| I | 8 | 2 | | V | 2 | 11 |
| J | 2 | 13 | | W | 2 | 10 |
| K | 2 | 8 | | X | 2 | 12 |
| L | 4 | 3 | | Y | 4 | 4 |
| M | 2 | 5 | | Z | 2 | 14 |

**Double-letter cards:**
| Card | Count | Points |
|------|-------|--------|
| QU | 2 | 9 |
| IN | 2 | 7 |
| ER | 2 | 7 |
| TH | 2 | 9 |
| CL | 2 | 10 |

---

## 8. Directory Structure

```
quiddler-webapp/
├── apps/
│   ├── web/                    # React frontend
│   │   ├── src/
│   │   │   ├── components/
│   │   │   │   ├── Card/
│   │   │   │   ├── Hand/
│   │   │   │   ├── GameBoard/
│   │   │   │   ├── Lobby/
│   │   │   │   └── ui/         # Shared UI components
│   │   │   ├── hooks/
│   │   │   ├── stores/         # Zustand stores
│   │   │   ├── services/       # API & Socket clients
│   │   │   ├── pages/
│   │   │   └── types/
│   │   └── package.json
│   │
│   └── server/                 # Node.js backend
│       ├── src/
│       │   ├── controllers/
│       │   ├── services/
│       │   │   ├── auth.ts
│       │   │   ├── lobby.ts
│       │   │   ├── game.ts
│       │   │   └── dictionary.ts
│       │   ├── game/
│       │   │   ├── engine.ts   # Core game logic
│       │   │   ├── deck.ts
│       │   │   ├── scoring.ts
│       │   │   └── validation.ts
│       │   ├── socket/
│       │   │   └── handlers.ts
│       │   ├── db/
│       │   │   └── schema.prisma
│       │   └── types/
│       └── package.json
│
├── packages/
│   └── shared/                 # Shared types and constants
│       ├── src/
│       │   ├── types.ts
│       │   ├── constants.ts
│       │   └── cardData.ts
│       └── package.json
│
├── turbo.json                  # Turborepo config
├── package.json
└── README.md
```

---

## 9. API Endpoints

### REST API

```
Authentication:
POST   /api/auth/register     - Create account
POST   /api/auth/login        - Login, receive JWT
POST   /api/auth/logout       - Invalidate session
GET    /api/auth/me           - Get current user

Lobby:
POST   /api/games             - Create new game room
GET    /api/games             - List public game rooms
GET    /api/games/:id         - Get game info
POST   /api/games/:id/join    - Join a game room

Users:
GET    /api/users/:id/stats   - Get player statistics
GET    /api/users/:id/history - Get game history
```

### WebSocket Events
See Section 6 for socket event definitions.

---

## 10. Minimum Viable Product (MVP)

For initial release, prioritize:

1. ✅ User registration/login
2. ✅ Create/join game rooms (2-6 players)
3. ✅ Full 8-round game with correct rules
4. ✅ Real-time multiplayer synchronization
5. ✅ Word validation against dictionary
6. ✅ Score tracking with bonuses
7. ✅ Basic responsive UI

**Post-MVP enhancements:**
- Challenge system
- Turn timers
- Player statistics/leaderboards
- Spectator mode
- Custom game settings
- Friends list
- Mobile app (React Native)

---

## 11. Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Dictionary accuracy | High | Use established Scrabble dictionary |
| Player disconnection | Medium | Auto-reconnection, game state persistence |
| Cheating (hidden cards) | High | Server-authoritative state, never send other players' hands |
| Concurrent modifications | Medium | Optimistic locking, turn-based prevents most issues |
| Scale (many games) | Low (initially) | Redis for state, horizontal scaling later |

---

## 12. Testing Strategy

- **Unit Tests**: Game engine, scoring, validation (Jest/Vitest)
- **Integration Tests**: API endpoints, socket handlers
- **E2E Tests**: Full game flows (Playwright/Cypress)
- **Manual Testing**: UI/UX, edge cases, multiplayer scenarios

---

## 13. Next Steps

1. **Create new repository** for the project
2. **Set up monorepo structure** with Turborepo
3. **Scaffold backend** with Express + Socket.IO + TypeScript
4. **Scaffold frontend** with Vite + React + TypeScript
5. **Implement card deck** and basic game engine
6. **Build authentication** flow
7. **Create lobby system**
8. **Develop game UI** with card components
9. **Integrate multiplayer** real-time features
10. **Deploy** to hosting platform

---

## References

- [Quiddler Official Rules (PDF)](https://www.playmonster.com/wp-content/uploads/2019/09/Quiddler_RULES.pdf)
- [Quiddler Wikipedia](https://en.wikipedia.org/wiki/Quiddler)
- [Socket.IO Documentation](https://socket.io/docs/v4/)
- [React Documentation](https://react.dev/)
