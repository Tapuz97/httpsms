package com.httpsms.services

import android.app.*
import android.content.Context
import android.content.Intent
import android.os.IBinder
import android.os.Handler
import android.os.Looper
import android.widget.Toast
import androidx.work.Constraints
import androidx.work.ExistingWorkPolicy
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequest
import androidx.work.WorkManager
import androidx.work.workDataOf
import com.httpsms.Constants
import com.httpsms.MyFirebaseMessagingService.SendSmsWorker
import com.httpsms.HttpSmsApiService
import com.httpsms.MainActivity
import com.httpsms.R
import com.httpsms.Settings
import timber.log.Timber

class StickyNotificationService: Service() {
    private val pollingHandler = Handler(Looper.getMainLooper())
    private val pollingTask = object : Runnable {
        override fun run() {
            if (Settings.isLoggedIn(applicationContext)) {
                Thread { pollOutstandingMessages() }.start()
            }
            pollingHandler.postDelayed(this, 15_000)
        }
    }
    override fun onBind(intent: Intent?): IBinder? {
        Timber.d("Some component want to bind with the service [${intent?.action}]")
        return null
    }

    override fun onCreate() {
        Timber.d("The service has been created")
        super.onCreate()
        val notification = createNotification()
        startForeground(1, notification)
        pollingHandler.post(pollingTask)
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        Timber.d("onStartCommand executed with startId: $startId")
        // by returning this we make sure the service is restarted if the system kills the service
        return START_STICKY
    }

    override fun onDestroy() {
        pollingHandler.removeCallbacks(pollingTask)
        super.onDestroy()
        Timber.d("The service has been destroyed")
        Toast.makeText(this, "Service destroyed", Toast.LENGTH_SHORT).show()
    }

    private fun pollOutstandingMessages() {
        HttpSmsApiService.create(applicationContext).getOutstandingMessageIDs().forEach { messageID ->
            val constraints = Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build()
            val work = OneTimeWorkRequest.Builder(SendSmsWorker::class.java)
                .setConstraints(constraints)
                .setInputData(workDataOf(Constants.KEY_MESSAGE_ID to messageID))
                .addTag(messageID)
                .build()
            WorkManager.getInstance(applicationContext)
                .enqueueUniqueWork(messageID, ExistingWorkPolicy.KEEP, work)
        }
    }


    private fun createNotification(): Notification {
        val notificationChannelId = "sticky_notification_channel"

        // depending on the Android API that we're dealing with we will have
        // to use a specific method to create the notification
        val notificationManager = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        val channel = NotificationChannel(
            notificationChannelId,
            notificationChannelId,
            NotificationManager.IMPORTANCE_HIGH
        ).apply {
            enableVibration(false)
            setShowBadge(false)
        }
        notificationManager.createNotificationChannel(channel)

        val pendingIntent: PendingIntent = Intent(this, MainActivity::class.java).let {
                notificationIntent -> PendingIntent.getActivity(
            this,
            0,
            notificationIntent,
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        }

        val builder: Notification.Builder = Notification.Builder(
            this,
            notificationChannelId
        )

        return builder
            .setContentTitle("httpSMS Listener")
            .setContentText("httpSMS is listening for sent and received SMS messages in the background.")
            .setContentIntent(pendingIntent)
            .setOngoing(true)
            .setSmallIcon(R.drawable.ic_stat_name)
            .build()
    }
}
