#
# Regular cron jobs for the jchat package
#
0 4	* * *	root	[ -x /usr/bin/jchat_maintenance ] && /usr/bin/jchat_maintenance
