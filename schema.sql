create table events (
  id bigserial not null primary key,
  payload jsonb not null
);
