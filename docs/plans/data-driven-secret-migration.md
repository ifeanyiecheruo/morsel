for secret migrations lets follow a similar pattern to db

lets have a migrations folder with numerically increasing set of file  names NNN_iniital.secrets.txt

the file is a migration script, a line delimited text document with entries like

# comment rename secret
rename "from-secret-name" "to-secret-name"

# comment delete secret
delete "secret-name"
 
lets embed the migration folder in the secrets.go
lets have a parser in secret manager that parses a migration script and produces and array of migration structs
lets have a Migrate method loadMigrations  walk the embedded fs in order building a list of migration structs

lets call that method from Migrate and dispatch those migrations