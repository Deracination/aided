# Quiddler Multiplayer Web App - Implementation Plan

## Based on The Deck Framework

This implementation will use [The Deck](https://github.com/xajik/thedeck) as the foundation - an open-source Flutter-based multiplayer card game platform.

---

## 1. Project Overview

Reimplementation of the Quiddler card game as a real-time multiplayer application using The Deck framework. Quiddler is a word-building card game where players form words from letter cards across 8 rounds of increasing hand sizes (3-10 cards).

### Core Game Mechanics to Implement
- **Deck**: 118 cards with letters A-Z plus special double-letter cards (QU, IN, ER, TH, CL)
- **Rounds**: 8 rounds, starting with 3 cards, adding 1 each round up to 10
- **Turn Flow**: Draw from deck/discard → Form words → Discard one card
- **Going Out**: Player uses all cards in valid words, others get one more turn
- **Scoring**: Card point values, +10 bonus for most words, +10 for longest word
- **Word Validation**: Dictionary lookup, no proper nouns/prefixes/suffixes/abbreviations
- **Challenge System**: Players can challenge words for point penalties

---

## 2. The Deck Framework Overview

### Why The Deck?
The Deck provides a complete multiplayer card game infrastructure:
- ✅ Real-time multiplayer via Socket.IO
- ✅ Game room management (create/join/leave)
- ✅ Redux-based state management
- ✅ Cross-platform (iOS, Android, Web via Flutter)
- ✅ Extensible game architecture with abstract base classes
- ✅ Local persistence with ObjectBox
- ✅ User profiles and session management

### The Deck Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    THE DECK FRAMEWORK                            │
├─────────────────────────────────────────────────────────────────┤
│  thedeck_client          │  Flutter UI, Redux stores, screens   │
│  thedeck_server          │  Game logic, socket handlers         │
│  thedeck_common          │  Shared models, utilities            │
│  thedeck_server_app      │  Server application wrapper          │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    CORE ABSTRACTIONS                             │
├─────────────────────────────────────────────────────────────────┤
│  GameRoom                │  roomId, participants, board, details│
│  GameBoard               │  moves, gameField, players           │
│  GameMove (abstract)     │  Base class for game actions         │
│  GameField (abstract)    │  Base class for game state           │
│  Player (abstract)       │  Base class for player state         │
│  GameParticipant         │  userId, sessionId, isHost, profile  │
└─────────────────────────────────────────────────────────────────┘
```

### Game Flow in The Deck
```
Client A                    Server                    Client B
    │                          │                          │
    │──── Make Move ──────────▶│                          │
    │                          │── Validate & Apply ──────│
    │                          │── Update GameField ──────│
    │◀── Broadcast State ──────│────── Broadcast State ──▶│
    │                          │                          │
```

---

## 3. Technology Stack

### Inherited from The Deck
| Component | Technology | Purpose |
|-----------|------------|---------|
| Language | **Dart** | Primary development language |
| Framework | **Flutter** | Cross-platform UI (iOS, Android, Web) |
| State Management | **Redux** | Centralized, predictable state |
| Real-time | **Socket.IO** | Multiplayer communication |
| Local Storage | **ObjectBox** | User data, offline support |
| Architecture | **Clean Architecture** | Separation of concerns |

### Quiddler-Specific Additions
| Component | Technology | Purpose |
|-----------|------------|---------|
| Dictionary | **SOWPODS/TWL word list** | Word validation (bundled asset) |
| Word Lookup | **Trie data structure** | O(m) word validation |
| Letter Assets | **Custom SVG/PNG cards** | Quiddler-themed card designs |

### Target Platforms
- **Primary**: Web (Flutter Web)
- **Secondary**: iOS, Android (Flutter mobile)
- **Server**: Dart server (thedeck_server_app)

---

## 4. Implementation Strategy

### Approach: Fork and Extend
1. **Fork** The Deck repository
2. **Add** Quiddler as a new game type within the framework
3. **Implement** Quiddler-specific classes extending The Deck's abstractions
4. **Integrate** dictionary-based word validation
5. **Deploy** as web app with optional mobile builds

---

## 5. Quiddler-Specific Data Models

### Extending The Deck Abstractions

```dart
/// Quiddler-specific game field extending GameField
class QuiddlerGameField extends GameField {
  final List<QuiddlerCard> deck;
  final List<QuiddlerCard> discardPile;
  final int currentRound;           // 1-8
  final int cardsPerHand;           // 3-10 based on round
  final bool someoneWentOut;
  final String? goingOutPlayerId;
  final Set<String> playersWithFinalTurn;

  QuiddlerGameField({
    required this.deck,
    required this.discardPile,
    required this.currentRound,
    required this.cardsPerHand,
    this.someoneWentOut = false,
    this.goingOutPlayerId,
    this.playersWithFinalTurn = const {},
  });
}

/// Quiddler player state extending Player
class QuiddlerPlayer extends Player {
  final String odyserId;
  final String odysername;
  final List<QuiddlerCard> hand;
  final List<List<QuiddlerCard>> laidDownWords;  // Words formed when going out
  final int roundScore;
  final int totalScore;
  final bool hasDrawnThisTurn;
  final bool isConnected;

  QuiddlerPlayer({
    required this.id,
    required this.username,
    required this.hand,
    this.laidDownWords = const [],
    this.roundScore = 0,
    this.totalScore = 0,
    this.hasDrawnThisTurn = false,
    this.isConnected = true,
  });
}

/// Quiddler move types extending GameMove
abstract class QuiddlerMove extends GameMove {
  final String playerId;
  final DateTime timestamp;
}

class DrawMove extends QuiddlerMove {
  final DrawSource source;  // deck or discard
}

class FormWordsMove extends QuiddlerMove {
  final List<List<QuiddlerCard>> words;  // Words being laid down
}

class DiscardMove extends QuiddlerMove {
  final QuiddlerCard card;
}

class ChallengeMove extends QuiddlerMove {
  final String challengedPlayerId;
  final String challengedWord;
}

/// Quiddler card model
class QuiddlerCard {
  final String id;
  final String letters;     // 'A', 'QU', 'TH', etc.
  final int points;
  final bool isDoubleLetter;

  QuiddlerCard({
    required this.id,
    required this.letters,
    required this.points,
  }) : isDoubleLetter = letters.length > 1;
}

enum DrawSource { deck, discard }
```

### Round Scores Model

```dart
class RoundScore {
  final String playerId;
  final int wordPoints;       // Sum of card values in words
  final int unusedPenalty;    // Negative points for unused cards
  final int mostWordsBonus;   // +10 if applicable
  final int longestWordBonus; // +10 if applicable
  final int totalRoundScore;

  RoundScore({
    required this.playerId,
    required this.wordPoints,
    required this.unusedPenalty,
    this.mostWordsBonus = 0,
    this.longestWordBonus = 0,
  }) : totalRoundScore = max(0, wordPoints - unusedPenalty + mostWordsBonus + longestWordBonus);
}
```

---

## 6. Directory Structure (New Files)

Additions to The Deck repository structure:

```
thedeck/
├── lib/
│   ├── games/
│   │   └── quiddler/                    # NEW: Quiddler game module
│   │       ├── quiddler_game.dart       # Game registration
│   │       ├── models/
│   │       │   ├── quiddler_card.dart
│   │       │   ├── quiddler_player.dart
│   │       │   ├── quiddler_field.dart
│   │       │   └── quiddler_move.dart
│   │       ├── logic/
│   │       │   ├── quiddler_engine.dart    # Core game rules
│   │       │   ├── deck_generator.dart     # Card deck creation
│   │       │   ├── scoring.dart            # Score calculation
│   │       │   └── word_validator.dart     # Dictionary lookup
│   │       └── ui/
│   │           ├── quiddler_board.dart     # Main game screen
│   │           ├── quiddler_hand.dart      # Player hand display
│   │           ├── quiddler_card_widget.dart
│   │           ├── word_builder.dart       # Word formation UI
│   │           └── scoreboard.dart
│   │
│   └── redux/
│       └── quiddler/                    # NEW: Quiddler Redux state
│           ├── quiddler_state.dart
│           ├── quiddler_actions.dart
│           └── quiddler_reducer.dart
│
├── assets/
│   └── quiddler/                        # NEW: Quiddler assets
│       ├── cards/                       # Card images
│       └── dictionary/
│           └── sowpods.txt              # Word list (~267K words)
│
├── thedeck_server/
│   └── lib/
│       └── games/
│           └── quiddler/                # NEW: Server-side game logic
│               ├── quiddler_room.dart
│               ├── quiddler_handler.dart
│               └── quiddler_validator.dart
│
└── thedeck_common/
    └── lib/
        └── games/
            └── quiddler/                # NEW: Shared Quiddler types
                ├── quiddler_types.dart
                └── quiddler_constants.dart
```

---

## 7. Core Implementation Components

### 7.1 Game Engine (quiddler_engine.dart)

```dart
class QuiddlerEngine {
  final WordValidator wordValidator;

  /// Initialize a new game
  QuiddlerGameField initializeGame(List<String> playerIds) {
    final deck = DeckGenerator.createShuffledDeck();
    final hands = _dealCards(deck, playerIds, cardsPerRound: 3);
    // ...
  }

  /// Process a player's move
  MoveResult processMove(QuiddlerGameField field, QuiddlerMove move) {
    return switch (move) {
      DrawMove m => _processDraw(field, m),
      FormWordsMove m => _processFormWords(field, m),
      DiscardMove m => _processDiscard(field, m),
      ChallengeMove m => _processChallenge(field, m),
    };
  }

  /// Check if player can go out (all cards used in valid words)
  bool canGoOut(List<QuiddlerCard> hand, List<List<QuiddlerCard>> words) {
    final usedCards = words.expand((w) => w).toSet();
    return usedCards.length == hand.length &&
           words.every((word) => wordValidator.isValid(_cardsToString(word)));
  }

  /// Calculate round scores with bonuses
  List<RoundScore> calculateRoundScores(List<QuiddlerPlayer> players) {
    // Find most words and longest word for bonuses
    // Calculate scores per player
    // ...
  }
}
```

### 7.2 Deck Generator (deck_generator.dart)

```dart
class DeckGenerator {
  static const Map<String, CardData> cardDistribution = {
    'A':  CardData(count: 10, points: 2),
    'B':  CardData(count: 2,  points: 8),
    'C':  CardData(count: 2,  points: 8),
    'D':  CardData(count: 4,  points: 5),
    'E':  CardData(count: 12, points: 2),
    'F':  CardData(count: 2,  points: 6),
    'G':  CardData(count: 4,  points: 6),
    'H':  CardData(count: 2,  points: 7),
    'I':  CardData(count: 8,  points: 2),
    'J':  CardData(count: 2,  points: 13),
    'K':  CardData(count: 2,  points: 8),
    'L':  CardData(count: 4,  points: 3),
    'M':  CardData(count: 2,  points: 5),
    'N':  CardData(count: 6,  points: 2),
    'O':  CardData(count: 8,  points: 2),
    'P':  CardData(count: 2,  points: 6),
    'Q':  CardData(count: 2,  points: 15),
    'R':  CardData(count: 6,  points: 5),
    'S':  CardData(count: 4,  points: 3),
    'T':  CardData(count: 6,  points: 3),
    'U':  CardData(count: 6,  points: 4),
    'V':  CardData(count: 2,  points: 11),
    'W':  CardData(count: 2,  points: 10),
    'X':  CardData(count: 2,  points: 12),
    'Y':  CardData(count: 4,  points: 4),
    'Z':  CardData(count: 2,  points: 14),
    // Double-letter cards
    'QU': CardData(count: 2,  points: 9),
    'IN': CardData(count: 2,  points: 7),
    'ER': CardData(count: 2,  points: 7),
    'TH': CardData(count: 2,  points: 9),
    'CL': CardData(count: 2,  points: 10),
  };

  static List<QuiddlerCard> createShuffledDeck() {
    final deck = <QuiddlerCard>[];
    int cardId = 0;

    for (final entry in cardDistribution.entries) {
      for (int i = 0; i < entry.value.count; i++) {
        deck.add(QuiddlerCard(
          id: '${entry.key}_${cardId++}',
          letters: entry.key,
          points: entry.value.points,
        ));
      }
    }

    return deck..shuffle();
  }
}
```

### 7.3 Word Validator (word_validator.dart)

```dart
class WordValidator {
  late final Set<String> _dictionary;
  late final TrieNode _trie;  // For prefix checking (optional optimization)

  Future<void> initialize() async {
    final wordList = await rootBundle.loadString('assets/quiddler/dictionary/sowpods.txt');
    _dictionary = wordList.split('\n').map((w) => w.trim().toLowerCase()).toSet();
  }

  bool isValid(String word) {
    if (word.length < 2) return false;
    final normalized = word.toLowerCase();
    return _dictionary.contains(normalized);
  }

  /// Convert card sequence to word string
  static String cardsToWord(List<QuiddlerCard> cards) {
    return cards.map((c) => c.letters).join().toLowerCase();
  }
}
```

### 7.4 Socket Events

```dart
// Client → Server events
class QuiddlerSocketEvents {
  static const String draw = 'quiddler:draw';
  static const String formWords = 'quiddler:form_words';
  static const String discard = 'quiddler:discard';
  static const String challenge = 'quiddler:challenge';
  static const String goOut = 'quiddler:go_out';
}

// Server → Client events
class QuiddlerServerEvents {
  static const String stateUpdate = 'quiddler:state';
  static const String turnChange = 'quiddler:turn';
  static const String roundEnd = 'quiddler:round_end';
  static const String gameEnd = 'quiddler:game_end';
  static const String error = 'quiddler:error';
}
```

---

## 8. Implementation Phases

### Phase 1: Setup & Core Models
- [ ] Fork The Deck repository
- [ ] Set up Flutter development environment with FVM
- [ ] Create Quiddler directory structure
- [ ] Implement QuiddlerCard, QuiddlerPlayer, QuiddlerField models
- [ ] Implement QuiddlerMove types
- [ ] Add shared types to thedeck_common

### Phase 2: Game Engine
- [ ] Implement DeckGenerator with correct card distribution
- [ ] Implement WordValidator with dictionary loading
- [ ] Build QuiddlerEngine with game rules
- [ ] Implement scoring calculation with bonuses
- [ ] Add round progression logic (3→10 cards)
- [ ] Implement "going out" and final turn mechanics
- [ ] Write unit tests for game engine

### Phase 3: Server Integration
- [ ] Create QuiddlerRoom extending GameRoom
- [ ] Implement QuiddlerHandler for socket events
- [ ] Add server-side move validation
- [ ] Implement state synchronization
- [ ] Handle player disconnection/reconnection

### Phase 4: Client UI
- [ ] Design and implement QuiddlerCardWidget
- [ ] Build QuiddlerHand with card arrangement
- [ ] Create WordBuilder for forming words (drag-and-drop)
- [ ] Implement main QuiddlerBoard game screen
- [ ] Add discard pile and draw deck visualization
- [ ] Build Scoreboard component
- [ ] Implement round/game end dialogs

### Phase 5: Redux Integration
- [ ] Define QuiddlerState
- [ ] Create QuiddlerActions for all game events
- [ ] Implement QuiddlerReducer
- [ ] Connect UI to Redux store
- [ ] Handle optimistic updates

### Phase 6: Polish & Deploy
- [ ] Add card animations (deal, draw, discard)
- [ ] Implement challenge system
- [ ] Add turn timer (optional)
- [ ] Test multiplayer scenarios
- [ ] Build and deploy web version
- [ ] Test on iOS/Android (optional)
- [ ] Set up hosting for server component

---

## 9. Card Distribution Reference

Based on official Quiddler deck (118 cards):

### Single Letters
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

### Double-Letter Cards
| Card | Count | Points |
|------|-------|--------|
| QU | 2 | 9 |
| IN | 2 | 7 |
| ER | 2 | 7 |
| TH | 2 | 9 |
| CL | 2 | 10 |

**Total: 118 cards**

---

## 10. Game Rules Summary

### Setup
1. Shuffle deck of 118 letter cards
2. Deal 3 cards to each player (round 1)
3. Place remaining cards face-down as draw pile
4. Turn top card face-up to start discard pile

### Turn Flow
1. **Draw**: Take top card from draw pile OR discard pile
2. **Optional - Go Out**: If all cards form valid words, lay them down
3. **Discard**: Place one card on discard pile

### Going Out
- Must use ALL cards in valid words (min 2 letters each)
- Once someone goes out, each other player gets ONE more turn
- Other players can also go out on their final turn

### Scoring
- Cards in valid words: Add their point values
- Unused cards: Subtract their point values
- **Bonus +10**: Player with most words in the round
- **Bonus +10**: Player with longest word (by letters, not cards)
- Minimum round score: 0 (can't go negative)
- Ties for bonuses: No bonus awarded

### Rounds
- Round 1: 3 cards per player
- Round 2: 4 cards per player
- ...
- Round 8: 10 cards per player

### Winning
- After 8 rounds, highest total score wins

---

## 11. MVP Features

### Must Have
- [x] Join/create game rooms (2-8 players)
- [x] Full 8-round game with correct rules
- [x] Draw from deck or discard pile
- [x] Form words and go out
- [x] Word validation against dictionary
- [x] Score tracking with bonuses
- [x] Real-time multiplayer sync
- [x] Basic responsive UI

### Nice to Have (Post-MVP)
- [ ] Challenge system
- [ ] Turn timer
- [ ] Animated card movements
- [ ] Sound effects
- [ ] Player avatars
- [ ] Game history/statistics
- [ ] Spectator mode

---

## 12. Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| The Deck learning curve | Medium | Study existing games in codebase, follow patterns |
| Dictionary size (2MB+) | Low | Compress, lazy load, or use API |
| Flutter Web performance | Medium | Optimize renders, use CanvasKit |
| Word validation edge cases | Low | Use established Scrabble dictionary |
| Server hosting costs | Low | Start with free tier (Fly.io, Railway) |

---

## 13. Development Environment Setup

### Prerequisites
```bash
# Install Flutter Version Manager
dart pub global activate fvm

# Clone The Deck
git clone https://github.com/xajik/thedeck.git quiddler-app
cd quiddler-app

# Use correct Flutter version (check .fvmrc)
fvm install
fvm use

# Get dependencies
fvm flutter pub get

# Run the app
fvm flutter run -d chrome  # For web
fvm flutter run            # For connected device
```

### Build Commands
```bash
# Web build
fvm flutter build web --release

# Android
fvm flutter build appbundle --release

# iOS
fvm flutter build ipa --release
```

---

## 14. Next Steps

1. **Fork The Deck** repository → create `quiddler-app` repo
2. **Study The Deck** codebase - especially existing game implementations
3. **Create Quiddler models** in thedeck_common
4. **Implement game engine** with unit tests
5. **Build server handlers** for game logic
6. **Create UI components** for cards and board
7. **Integrate Redux** state management
8. **Test multiplayer** with multiple browsers
9. **Deploy** web version

---

## References

- [The Deck Repository](https://github.com/xajik/thedeck)
- [Quiddler Official Rules (PDF)](https://www.playmonster.com/wp-content/uploads/2019/09/Quiddler_RULES.pdf)
- [Quiddler Wikipedia](https://en.wikipedia.org/wiki/Quiddler)
- [Flutter Documentation](https://flutter.dev/docs)
- [Flutter Web](https://flutter.dev/web)
- [Socket.IO Dart Client](https://pub.dev/packages/socket_io_client)
