  mongoRootPassword=`date -u | md5`
  mongoDatabasePassword=`date -u | md5`

  pulumi stack select "dev-usw2"

  pulumi config set --secret nativelink-cloud:mongoRootPassword "${mongoRootPassword}"
  pulumi config set --secret nativelink-cloud:mongoDatabasePassword "${mongoDatabasePassword}"