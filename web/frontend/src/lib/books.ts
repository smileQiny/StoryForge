export type BooksResponse<T> =
  | ReadonlyArray<T>
  | { readonly books?: ReadonlyArray<T> | null }
  | null
  | undefined;

export function getBooksFromResponse<T>(payload: BooksResponse<T>): ReadonlyArray<T> {
  if (Array.isArray(payload)) {
    return payload;
  }
  return payload?.books ?? [];
}
