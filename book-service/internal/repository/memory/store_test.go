package memory_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"library_app/book-service/internal/domain"
	"library_app/book-service/internal/repository"
	"library_app/book-service/internal/repository/memory"
)

// Проверка контрактов на уровне самого хранилища: уникальность индексов,
// каскадное удаление, агрегаты и атомарность выдачи.

var (
	_ repository.BookRepository = (*memory.Store)(nil)
	_ repository.CopyRepository = (*memory.Store)(nil)
)

func newBook(t *testing.T, isbn, title string) *domain.Book {
	t.Helper()

	book, err := domain.NewBook(domain.NewBookParams{
		ISBN:          isbn,
		Title:         title,
		Author:        "Author",
		PublishedYear: 2020,
	})
	if err != nil {
		t.Fatalf("NewBook: unexpected error: %v", err)
	}

	return book
}

func TestBookCRUD(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := memory.NewStore()

	book := newBook(t, "978-0-13-419044-0", "Original")

	if err := store.Create(ctx, book); err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}

	// Повторный ISBN того же экземпляра допустим, чужого — нет.
	if err := store.Create(ctx, newBook(t, "978-0-13-419044-0", "Other")); !errors.Is(err, domain.ErrISBNAlreadyExists) {
		t.Fatalf("want %v, got %v", domain.ErrISBNAlreadyExists, err)
	}

	// Хранилище отдаёт копию: мутация результата не должна портить данные.
	fetched, err := store.GetByID(ctx, book.ID)
	if err != nil {
		t.Fatalf("GetByID: unexpected error: %v", err)
	}

	fetched.Title = "Mutated"

	again, err := store.GetByID(ctx, book.ID)
	if err != nil {
		t.Fatalf("GetByID: unexpected error: %v", err)
	}

	if again.Title != "Original" {
		t.Fatalf("stored data mutated through returned copy: %q", again.Title)
	}

	byISBN, err := store.GetByISBN(ctx, book.ISBN)
	if err != nil {
		t.Fatalf("GetByISBN: unexpected error: %v", err)
	}

	if byISBN.ID != book.ID {
		t.Fatalf("ISBN index points to %s, want %s", byISBN.ID, book.ID)
	}

	// Обновление несуществующей книги.
	if err := store.Update(ctx, newBook(t, "978-0-306-40615-7", "Ghost")); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want %v, got %v", domain.ErrNotFound, err)
	}

	title := "Updated"
	if err := again.Apply(domain.BookUpdate{Title: &title}); err != nil {
		t.Fatalf("Apply: unexpected error: %v", err)
	}

	if err := store.Update(ctx, again); err != nil {
		t.Fatalf("Update: unexpected error: %v", err)
	}

	updated, err := store.GetByID(ctx, book.ID)
	if err != nil {
		t.Fatalf("GetByID: unexpected error: %v", err)
	}

	if updated.Title != "Updated" {
		t.Fatalf("update lost: %q", updated.Title)
	}

	if err := store.Delete(ctx, book.ID); err != nil {
		t.Fatalf("Delete: unexpected error: %v", err)
	}

	if _, err := store.GetByID(ctx, book.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want %v, got %v", domain.ErrNotFound, err)
	}

	if err := store.Delete(ctx, book.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second delete must report not found, got %v", err)
	}
}

func TestListPagination(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := memory.NewStore()

	for i := range 5 {
		book := newBook(t, isbn13(i), fmt.Sprintf("Book %d", i))
		if err := store.Create(ctx, book); err != nil {
			t.Fatalf("Create: unexpected error: %v", err)
		}
	}

	books, total, err := store.List(ctx, domain.BookFilter{Limit: 2, Offset: 1})
	if err != nil {
		t.Fatalf("List: unexpected error: %v", err)
	}

	if total != 5 {
		t.Fatalf("total must ignore paging, got %d", total)
	}

	if len(books) != 2 {
		t.Fatalf("expected page of 2, got %d", len(books))
	}

	// Страница не должна пересекаться с первой.
	first, _, err := store.List(ctx, domain.BookFilter{Limit: 1})
	if err != nil {
		t.Fatalf("List: unexpected error: %v", err)
	}

	for _, book := range books {
		if book.ID == first[0].ID {
			t.Fatal("offset paging returned an item from the first page")
		}
	}

	books, total, err = store.List(ctx, domain.BookFilter{Query: "Book 3"})
	if err != nil {
		t.Fatalf("List: unexpected error: %v", err)
	}

	if total != 1 || len(books) != 1 || books[0].Title != "Book 3" {
		t.Fatalf("query filter broken: total=%d books=%d", total, len(books))
	}
}

func TestCopyInventory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := memory.NewStore()

	book := newBook(t, "978-0-13-419044-0", "Catalog")
	if err := store.Create(ctx, book); err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}

	// Экземпляр несуществующей книги.
	if err := store.CreateCopy(ctx, &domain.Copy{ID: uuid.New(), BookID: uuid.New(), Barcode: "X"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want %v, got %v", domain.ErrNotFound, err)
	}

	var ids []uuid.UUID

	// Порядок выдачи определяется временем регистрации, поэтому задаём его явно:
	// time.Now() в домене имеет разрешение часов ОС и в быстрой цикле даёт коллизии.
	base := time.Now().UTC()

	for i := range 3 {
		item, err := domain.NewCopy(book.ID, fmt.Sprintf("BC-%03d", i))
		if err != nil {
			t.Fatalf("NewCopy: unexpected error: %v", err)
		}

		item.CreatedAt = base.Add(time.Duration(i) * time.Minute)

		if err := store.CreateCopy(ctx, item); err != nil {
			t.Fatalf("CreateCopy: unexpected error: %v", err)
		}

		ids = append(ids, item.ID)
	}

	// Дубликат штрихкода.
	dup, err := domain.NewCopy(book.ID, "BC-000")
	if err != nil {
		t.Fatalf("NewCopy: unexpected error: %v", err)
	}

	if err := store.CreateCopy(ctx, dup); !errors.Is(err, domain.ErrBarcodeAlreadyExists) {
		t.Fatalf("want %v, got %v", domain.ErrBarcodeAlreadyExists, err)
	}

	stats, err := store.Stats(ctx, book.ID)
	if err != nil {
		t.Fatalf("Stats: unexpected error: %v", err)
	}

	if stats.Total != 3 || stats.Available != 3 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	if err := store.UpdateStatus(ctx, ids[2], domain.CopyStatusLost); err != nil {
		t.Fatalf("UpdateStatus: unexpected error: %v", err)
	}

	if err := store.UpdateStatus(ctx, ids[2], domain.CopyStatus("BROKEN")); !errors.Is(err, domain.ErrInvalidCopyStatus) {
		t.Fatalf("want %v, got %v", domain.ErrInvalidCopyStatus, err)
	}

	stats, err = store.Stats(ctx, book.ID)
	if err != nil {
		t.Fatalf("Stats: unexpected error: %v", err)
	}

	// Потерянный экземпляр учитывается в Total, но не в Available.
	if stats.Total != 3 || stats.Available != 2 {
		t.Fatalf("lost copy must stay out of Available: %+v", stats)
	}

	copies, err := store.ListByBook(ctx, book.ID)
	if err != nil {
		t.Fatalf("ListByBook: unexpected error: %v", err)
	}

	if len(copies) != 3 {
		t.Fatalf("expected 3 copies, got %d", len(copies))
	}

	// Выдача идёт по одному, последний доступный — BC-001 (BC-002 потерян).
	first, err := store.AcquireAvailable(ctx, book.ID)
	if err != nil {
		t.Fatalf("AcquireAvailable: unexpected error: %v", err)
	}

	if first.Barcode != "BC-000" {
		t.Fatalf("expected oldest available BC-000, got %s", first.Barcode)
	}

	second, err := store.AcquireAvailable(ctx, book.ID)
	if err != nil {
		t.Fatalf("AcquireAvailable: unexpected error: %v", err)
	}

	if second.Barcode != "BC-001" {
		t.Fatalf("expected BC-001, got %s", second.Barcode)
	}

	if _, err := store.AcquireAvailable(ctx, book.ID); !errors.Is(err, domain.ErrNoAvailableCopies) {
		t.Fatalf("want %v, got %v", domain.ErrNoAvailableCopies, err)
	}

	// Каскад: удаление книги забирает её экземпляры.
	if err := store.Delete(ctx, book.ID); err != nil {
		t.Fatalf("Delete: unexpected error: %v", err)
	}

	if _, err := store.GetCopyByID(ctx, ids[0]); !errors.Is(err, domain.ErrCopyNotFound) {
		t.Fatalf("copies must be removed with the book, got %v", err)
	}
}

func TestAcquireAvailableIsAtomic(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := memory.NewStore()

	book := newBook(t, "978-0-13-419044-0", "Concurrency")
	if err := store.Create(ctx, book); err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}

	const copies = 8

	for i := range copies {
		item, err := domain.NewCopy(book.ID, fmt.Sprintf("BC-%03d", i))
		if err != nil {
			t.Fatalf("NewCopy: unexpected error: %v", err)
		}

		if err := store.CreateCopy(ctx, item); err != nil {
			t.Fatalf("CreateCopy: unexpected error: %v", err)
		}
	}

	// Параллельная выдача не должна отдавать один экземпляр дважды.
	const workers = 16

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		taken = make(map[uuid.UUID]int)
	)

	for range workers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			item, err := store.AcquireAvailable(ctx, book.ID)

			mu.Lock()
			defer mu.Unlock()

			if err == nil {
				taken[item.ID]++
			}
		}()
	}

	wg.Wait()

	if len(taken) != copies {
		t.Fatalf("expected %d distinct copies handed out, got %d", copies, len(taken))
	}

	for id, count := range taken {
		if count != 1 {
			t.Fatalf("copy %s handed out %d times", id, count)
		}
	}

	stats, err := store.Stats(ctx, book.ID)
	if err != nil {
		t.Fatalf("Stats: unexpected error: %v", err)
	}

	if stats.Available != 0 || stats.OnLoan != copies {
		t.Fatalf("expected all copies on loan: %+v", stats)
	}
}

// isbn13 собирает валидный ISBN-13 по счётчику: 12 цифр + контрольная.
func isbn13(seq int) string {
	base := fmt.Sprintf("978%09d", seq)

	sum := 0
	for i := range len(base) {
		digit := int(base[i] - '0')
		if i%2 == 1 {
			digit *= 3
		}

		sum += digit
	}

	return base + fmt.Sprintf("%d", (10-sum%10)%10)
}
