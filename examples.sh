#!/bin/bash

echo "=== Zero-SQL Examples ==="
echo

echo "1. Simple SELECT with WHERE clause:"
./zero-sql "SELECT name, age FROM users WHERE age > 18"

echo

echo "2. SELECT with LIKE pattern matching:"
./zero-sql "SELECT name FROM users WHERE email LIKE '%@gmail.com'"
echo

echo "3. Complex WHERE with AND/OR:"
./zero-sql "SELECT name FROM users WHERE (age > 18 AND status = 'active') OR name LIKE 'John%'"
echo

echo "4. SELECT with IN operator:"
./zero-sql "SELECT name FROM users WHERE status IN ('active', 'pending')"
echo

echo "5. SELECT with BETWEEN operator:"
./zero-sql "SELECT name FROM products WHERE price BETWEEN 10 AND 100"
echo

echo "6. SELECT with ORDER BY and LIMIT:"
./zero-sql "SELECT name, age FROM users ORDER BY age DESC LIMIT 5"
echo

echo "7. GROUP BY with COUNT aggregation:"
./zero-sql "SELECT status, COUNT(*) as total FROM users GROUP BY status"
echo

echo "8. Complex e-commerce query:"
./zero-sql "SELECT name, price, category FROM products WHERE price BETWEEN 10 AND 100 AND category IN ('electronics', 'books') ORDER BY price ASC LIMIT 20"
echo

echo "9. Boolean values and complex conditions:"
./zero-sql "SELECT title, author FROM articles WHERE published = true AND title LIKE '%mongodb%'"
echo

echo "10. IS NULL check:"
./zero-sql "SELECT name FROM users WHERE deleted_at IS NULL"
echo

echo "11. Simple INNER JOIN:"
./zero-sql "SELECT u.name, p.title FROM users u JOIN posts p ON u.id = p.user_id"
echo

echo "12. LEFT JOIN:"
./zero-sql "SELECT u.name, p.title FROM users u LEFT JOIN posts p ON u.id = p.user_id"
echo

echo "13. JOIN with WHERE conditions:"
./zero-sql "SELECT u.name, p.title FROM users u JOIN posts p ON u.id = p.user_id WHERE u.active = true AND p.published = true"
echo

echo "14. Multiple JOINs:"
./zero-sql "SELECT u.name, p.title, c.name as category FROM users u JOIN posts p ON u.id = p.user_id JOIN categories c ON p.category_id = c.id"
echo

echo "15. Complex JOIN with aggregation:"
./zero-sql "SELECT u.name, COUNT(p.id) as post_count FROM users u LEFT JOIN posts p ON u.id = p.user_id GROUP BY u.id, u.name"
echo

echo "=== End of Examples ===" 